package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	// 1. Make a GET request
	resp, err := http.Get("http://localhost:8081/raft-test")
	if err != nil {
		log.Fatal(err)
	}

	// 2. CRITICAL: Always close the body when done!
	defer resp.Body.Close()

	// 3. Read the answer
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Server said: %s\n", string(body))
}
