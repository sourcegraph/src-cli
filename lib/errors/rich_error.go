package errors

import (
	"reflect"
	"slices"
	"sync"

	"go.opentelemetry.io/otel/attribute"
)

type RichError interface {
	// Unpack extracts the underlying error and attributes from a RichError.
	//
	// Unpacking should only return attributes one-level deep; a caller may
	// optionally continue recursively unpacking the underlying error.
	Unpack() RichErrorData
}

type RichErrorData struct {
	Err   error
	Attrs []attribute.KeyValue
}

type PtrRichError[A any] interface {
	*A
	RichError
}

// NewRichError is a helper function for wrapping custom errors
// for the observation package.
//
// Pre-condition: The outermost level of ptr must be non-nil.
//
// Generally, this is easy to satisfy by using & on a named return value.
//
// The type signature is more complicated than usual to avoid an extra
// level of pointer indirection when the type implementing RichError uses
// a pointer receiver for RichError.Unpack. See rich_error_test.go for
// examples.
func NewRichError[A any, T PtrRichError[A]](ptr2 **A) *RichError {
	v := RichError(NewErrorPtr[A, T](ptr2))
	return &v
}

func UnpackRichErrorPtr(richErr *RichError) (error, []attribute.KeyValue) {
	if richErr == nil || *richErr == nil {
		return nil, nil
	}
	if val := reflect.ValueOf(*richErr); val.Kind() == reflect.Ptr && val.IsNil() {
		return nil, nil
	}
	data := (*richErr).Unpack()
	return data.Err, data.Attrs
}

type ErrorPtr[A any, T PtrRichError[A]] struct {
	ptr2 **A
}

// nolint:unused
func typeAssertion[A any, T PtrRichError[A]]() {
	var _ RichError = (*ErrorPtr[A, T])(nil)
}

// NewErrorPtr is the only way to create an ErrorPtr.
func NewErrorPtr[A any, T PtrRichError[A]](ptr2 **A) ErrorPtr[A, T] {
	if ptr2 == nil {
		panic("Expected non-nil pointer to some error type")
	}
	return ErrorPtr[A, T]{ptr2: ptr2}
}

func (p ErrorPtr[A, T]) Unpack() RichErrorData {
	var empty RichErrorData
	if p.ptr2 == nil {
		panic("Incorrectly created ErrorPtr without going through NewErrorPtr")
	}
	ptr := *p.ptr2
	if ptr == nil {
		return RichErrorData{Err: nil, Attrs: nil}
	}
	// We need this dynamic type checks here because Go doesn't allow
	// constraints on type parameters in methods, and we want to
	// have a method so that we can satisfy the RichError interface.
	val, ok := any(ptr).(RichError)
	if !ok {
		// We need two different type checks here because RichError may be
		// implemented with a struct receiver or a pointer receiver.
		// See the tests for more details.
		if val, ok = any(*ptr).(RichError); !ok {
			panic("Dynamic type check for pointer type failed; did you create ErrorPtr without using NewErrorPtr?")
		}
	}
	if val == nil {
		return empty
	}
	if inner := reflect.ValueOf(val); inner.Kind() == reflect.Ptr && inner.IsNil() {
		return empty
	}
	return val.Unpack()
}

// ErasedErrorPtr stores a type-erased 'error' instead of
// generic **T like ErrorPtr.
type ErasedErrorPtr struct {
	Err *error
}

var _ RichError = ErasedErrorPtr{}

func (e ErasedErrorPtr) Unpack() RichErrorData {
	if e.Err == nil || *e.Err == nil {
		return RichErrorData{Err: nil, Attrs: nil}
	}
	if re := RichError(nil); AsInterface(*e.Err, &re) {
		return re.Unpack()
	}
	return RichErrorData{Err: *e.Err, Attrs: nil}
}

// Collector represents multiple errors and additional log fields that arose from those errors.
// This type is thread-safe.
type Collector struct {
	mu         sync.Mutex
	errs       error
	extraAttrs []attribute.KeyValue
}

var _ RichError = (*Collector)(nil)

func NewErrorCollector() *Collector { return &Collector{errs: nil} }

func (e *Collector) Collect(err *error, attrs ...attribute.KeyValue) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err != nil && *err != nil {
		e.errs = Append(e.errs, *err)
		e.extraAttrs = append(e.extraAttrs, attrs...)
	}
}

func (e *Collector) Error() string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.errs == nil {
		return ""
	}
	return e.errs.Error()
}

func (e *Collector) Unwrap() error {
	// Collector wraps collected errors, for compatibility with errors.HasType,
	// errors.Is etc it has to implement Unwrap to return the inner errors the
	// collector stores.
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.errs
}

func (e *Collector) Unpack() RichErrorData {
	if e.Error() == "" {
		return RichErrorData{Err: nil, Attrs: nil}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// Explicitly clone slice for 1 level of thread safety.
	return RichErrorData{Err: e.errs, Attrs: slices.Clone(e.extraAttrs)}
}
