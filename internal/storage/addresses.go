package storage

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func ReadAddresses(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open addresses %s: %w", path, err)
	}
	defer file.Close()

	var addresses []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		address := strings.TrimSpace(scanner.Text())
		if address != "" {
			addresses = append(addresses, address)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read addresses %s: %w", path, err)
	}
	return addresses, nil
}
