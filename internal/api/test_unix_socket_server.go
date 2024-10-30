package api

import (
	"fmt"
	"net"
	"os"
	"sync"
)

type Server struct {
	Listener net.Listener
	StopChan chan struct{}
	Wg       sync.WaitGroup
}

func CreateTempFile(dir, prefix, suffix string) (string, error) {
	// Get the default temporary directory for the OS
	if dir == "" {
		dir = os.TempDir()
	}

	// Create a temporary file with the specified prefix and suffix
	tempFile, err := os.CreateTemp(dir, prefix+"*"+suffix)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Close the file to release the file descriptor
	tempFile.Close()

	// Return the name of the created file
	return tempFile.Name(), nil
}

func createUnixSocket(socketPath string) (net.Listener, error) {
	// Clean up the socket file if it already exists
	_ = os.Remove(socketPath)

	// Create a Unix domain socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Unix domain socket: %w", err)
	}

	return listener, nil
}

// StartServer starts the server in a goroutine and returns a stop function
func StartUnixSocketServer(socketPath string) (*Server, error) {
	listener, err := createUnixSocket(socketPath)
	if err != nil {
		return nil, err
	}

	server := &Server{
		Listener: listener,
		StopChan: make(chan struct{}),
	}

	server.Wg.Add(1)
	go func() {
		defer server.Wg.Done()
		for {
			conn, err := server.Listener.Accept()
			if err != nil {
				select {
				case <-server.StopChan:
					return
				default:
					fmt.Println("Error accepting connection:", err)
					continue
				}
			}

			// Handle each connection in a separate goroutine
			server.Wg.Add(1)
			go func(conn net.Conn) {
				defer server.Wg.Done()
				defer conn.Close()

				// Handle incoming data from the connection
				buf := make([]byte, 1024)
				n, err := conn.Read(buf)
				if err != nil {
					fmt.Println("Error reading from connection:", err)
					return
				}

				fmt.Printf("Received: %s\n", string(buf[:n]))

				// Example: Write a response back
				_, err = conn.Write([]byte("Hello from server\n"))
				if err != nil {
					fmt.Println("Error writing to connection:", err)
					return
				}
			}(conn)
		}
	}()

	return server, nil
}

// StopServer stops the server and waits for all goroutines to finish
func (s *Server) Stop() {
	close(s.StopChan)
	s.Listener.Close()
	s.Wg.Wait()
}
