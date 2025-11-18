package errors

import (
	"reflect"

	"github.com/xlab/treeprint"
)

type TreeNode struct {
	TypeName string
	Children []*TreeNode
}

func (n *TreeNode) String() string {
	return debugTreeRecursive(n).String()
}

func debugTreeRecursive(n *TreeNode) treeprint.Tree {
	if n == nil {
		return treeprint.NewWithRoot("<nil>")
	}
	t := treeprint.NewWithRoot(n.TypeName)
	for _, child := range n.Children {
		t.AddNode(debugTreeRecursive(child))
	}
	return t
}

// UnpackTree returns a tree of error types and their children.
//
// This function is meant for debugging issues when you're trying to write
// a test for error handling, and want to inspect the structure of an error
// tree.
//
// DO NOT use this function for matching errors -- use errors.Is, errors.As etc.
func UnpackTree(err error) *TreeNode {
	if err == nil {
		return nil
	}
	typeName := reflect.TypeOf(err).String()
	children := []*TreeNode{}
	if e, ok := err.(interface{ Unwrap() []error }); ok {
		for _, suberr := range e.Unwrap() {
			children = append(children, UnpackTree(suberr))
		}
	} else if e, ok := err.(MultiError); ok {
		for _, suberr := range e.Errors() {
			children = append(children, UnpackTree(suberr))
		}
	} else if e, ok := err.(Wrapper); ok {
		children = append(children, UnpackTree(e.Unwrap()))
	} else if e, ok := err.(interface{ Cause() error }); ok {
		children = append(children, UnpackTree(e.Cause()))
	}
	return &TreeNode{typeName, children}
}
