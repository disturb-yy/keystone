package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
)

const maxMetadataBytes = 1 << 20

func readRuntimeMetadata(paths localstate.Paths) (localstate.Metadata, error) {
	data, err := os.ReadFile(paths.MetadataPath)
	if os.IsNotExist(err) {
		return localstate.Metadata{}, newCLIError(ErrorMetadataMissing, "RuntimeMetadata 不存在", err)
	}
	if err != nil {
		return localstate.Metadata{}, newCLIError(ErrorMetadataInvalid, "读取 RuntimeMetadata 失败", err)
	}
	metadata, err := decodeMetadata(data)
	if err != nil {
		return localstate.Metadata{}, newCLIError(ErrorMetadataInvalid, "RuntimeMetadata JSON 无效", err)
	}
	if err := validateMetadata(metadata); err != nil {
		return localstate.Metadata{}, newCLIError(ErrorMetadataInvalid, "RuntimeMetadata 字段无效", err)
	}
	return metadata, nil
}

func decodeMetadata(data []byte) (localstate.Metadata, error) {
	if len(data) > maxMetadataBytes {
		return localstate.Metadata{}, fmt.Errorf("metadata exceeds %d bytes", maxMetadataBytes)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	var metadata localstate.Metadata
	if err := decoder.Decode(&metadata); err != nil {
		return localstate.Metadata{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return localstate.Metadata{}, errors.New("metadata contains multiple JSON values")
		}
		return localstate.Metadata{}, err
	}
	return metadata, nil
}

func validateMetadata(metadata localstate.Metadata) error {
	if strings.TrimSpace(metadata.InstanceID) == "" {
		return errors.New("instance_id is required")
	}
	if err := validateDaemonEndpoint(metadata.Endpoint); err != nil {
		return fmt.Errorf("endpoint is invalid: %w", err)
	}
	return nil
}

func validateDaemonEndpoint(endpoint string) error {
	if strings.TrimSpace(endpoint) != endpoint || endpoint == "" {
		return errors.New("endpoint must be a non-empty trimmed host:port")
	}
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("split endpoint host and port: %w", err)
	}
	if host != "127.0.0.1" {
		return fmt.Errorf("endpoint host %q is not IPv4 loopback", host)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("endpoint port %q is invalid", portText)
	}
	return nil
}
