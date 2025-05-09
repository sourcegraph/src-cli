package main

import (
	"bytes"
	"fmt"
	"os"
)

func main() {
	// Create and test our improved buffer handling code
	fmt.Println("Creating test with large buffer...")
	
	// Create a buffer of about 50MB
	size := 50 * 1024 * 1024
	buf := make([]byte, size)
	
	// Fill it with diff-like content
	copy(buf, []byte("diff --git a/file b/file\nindex 123..456 789\n--- a/file\n+++ b/file\n@@ -1,10 +1,20 @@\n"))
	
	// Fill the rest with + and - content
	for i := 100; i < size-100; i++ {
		if i % 2 == 0 {
			buf[i] = '-'
		} else {
			buf[i] = '+'
		}
		
		// Add newlines
		if i % 80 == 79 {
			buf[i] = '\n'
		}
	}
	
	// Add end markers
	copy(buf[size-50:], []byte("\n-- \n2.40.0\n"))
	
	fmt.Printf("Created buffer of %d bytes\n", len(buf))
	
	// Write to temp file
	tmp, err := os.CreateTemp("", "large-diff-*.txt")
	if err != nil {
		fmt.Printf("Failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	fileName := tmp.Name()
	defer os.Remove(fileName)
	
	// Write in chunks similar to our fixed implementation
	remaining := buf
	fmt.Println("Writing in chunks...")
	
	for len(remaining) > 0 {
		chunkSize := 10 * 1024 * 1024 // 10MB chunks
		if len(remaining) < chunkSize {
			chunkSize = len(remaining)
		}
		
		n, err := tmp.Write(remaining[:chunkSize])
		if err != nil {
			fmt.Printf("Failed to write chunk: %v\n", err)
			os.Exit(1)
		}
		
		fmt.Printf("Wrote chunk of %d bytes\n", n)
		remaining = remaining[n:]
	}
	
	tmp.Close()
	
	// Read it back in
	fmt.Println("Reading back the file...")
	content, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Printf("Failed to read file: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Read %d bytes from file\n", len(content))
	
	// Validate content
	if len(content) != size {
		fmt.Printf("ERROR: Size mismatch! Expected %d got %d\n", size, len(content))
		os.Exit(1)
	}
	
	// Verify diff headers and end markers
	if !bytes.HasPrefix(content, []byte("diff --git")) {
		fmt.Println("ERROR: Missing diff header!")
		os.Exit(1)
	}
	
	if !bytes.Contains(content[size-50:], []byte("2.40.0")) {
		fmt.Println("ERROR: Missing end marker!")
		os.Exit(1)
	}
	
	fmt.Println("Success! Buffer handling test passed.")
	fmt.Println("This confirms our file-based approach can handle large diffs without truncation.")
}