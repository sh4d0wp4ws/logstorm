package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type strictString string

func (value *strictString) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("expected a string")
	}
	*value = strictString(node.Value)
	return nil
}

type Config struct {
	Streams []StreamConfig `yaml:"streams"`
}

type StreamConfig struct {
	Name     strictString  `yaml:"name"`
	Format   strictString  `yaml:"format"`
	Type     strictString  `yaml:"type"`
	Target   strictString  `yaml:"target"`
	Number   *int          `yaml:"number"`
	Loop     bool          `yaml:"loop"`
	Delay    *strictString `yaml:"delay"`
	EPS      *int          `yaml:"eps"`
	Duration *strictString `yaml:"duration"`
}

type Stream struct {
	Name   string
	Option *Option
}

func LoadStreams(path string) ([]Stream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("config must contain exactly one YAML document")
		}
		return nil, fmt.Errorf("decode config: %w", err)
	}

	return config.streams()
}

func (config Config) streams() ([]Stream, error) {
	if len(config.Streams) == 0 {
		return nil, fmt.Errorf("config must contain at least one stream")
	}

	streams := make([]Stream, 0, len(config.Streams))
	names := make(map[string]struct{}, len(config.Streams))
	for _, streamConfig := range config.Streams {
		stream, err := streamConfig.stream()
		if err != nil {
			return nil, err
		}
		if _, found := names[stream.Name]; found {
			return nil, fmt.Errorf("stream %q is defined more than once", stream.Name)
		}
		names[stream.Name] = struct{}{}
		streams = append(streams, stream)
	}
	return streams, nil
}

func (streamConfig StreamConfig) stream() (Stream, error) {
	name := strings.TrimSpace(string(streamConfig.Name))
	if name == "" {
		return Stream{}, fmt.Errorf("stream name is required")
	}

	option := defaultOptions()
	var err error
	format := strings.TrimSpace(string(streamConfig.Format))
	if format == "" {
		return Stream{}, fmt.Errorf("stream %q: format is required", name)
	}
	if option.Format, err = ParseFormat(format); err != nil {
		return Stream{}, fmt.Errorf("stream %q: %w", name, err)
	}
	logType := strings.TrimSpace(string(streamConfig.Type))
	if logType == "" {
		return Stream{}, fmt.Errorf("stream %q: type is required", name)
	}
	if option.Type, err = ParseType(logType); err != nil {
		return Stream{}, fmt.Errorf("stream %q: %w", name, err)
	}
	if !isNetworkOutput(option.Type) {
		return Stream{}, fmt.Errorf("stream %q: type must be tcp or udp", name)
	}
	if option.Target, err = ParseTarget(option.Type, string(streamConfig.Target)); err != nil {
		return Stream{}, fmt.Errorf("stream %q: %w", name, err)
	}
	if err := validateNetworkTarget(option.Target); err != nil {
		return Stream{}, fmt.Errorf("stream %q: %w", name, err)
	}
	if streamConfig.Number != nil {
		if option.Number, err = ParseNumber(*streamConfig.Number); err != nil {
			return Stream{}, fmt.Errorf("stream %q: %w", name, err)
		}
	}
	if streamConfig.Number != nil && streamConfig.Loop {
		return Stream{}, fmt.Errorf("stream %q: number and loop cannot both be set", name)
	}
	if streamConfig.EPS != nil && streamConfig.Delay != nil {
		return Stream{}, fmt.Errorf("stream %q: eps and delay cannot both be set", name)
	}
	if streamConfig.Delay != nil {
		if option.Delay, err = ParseDelay(string(*streamConfig.Delay)); err != nil {
			return Stream{}, fmt.Errorf("stream %q: invalid delay: %w", name, err)
		}
	}
	if streamConfig.EPS != nil {
		if option.EPS, err = parseEPS(*streamConfig.EPS); err != nil {
			return Stream{}, fmt.Errorf("stream %q: %w", name, err)
		}
	}
	if streamConfig.Duration != nil {
		if option.Duration, err = parseDuration(string(*streamConfig.Duration)); err != nil {
			return Stream{}, fmt.Errorf("stream %q: invalid duration: %w", name, err)
		}
	}
	option.Forever = streamConfig.Loop || (streamConfig.Duration != nil && streamConfig.Number == nil)
	return Stream{Name: name, Option: option}, nil
}

func parseEPS(eps int) (int, error) {
	if eps <= 0 {
		return 0, fmt.Errorf("eps must be positive")
	}
	return eps, nil
}

func parseDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return duration, nil
}

func validateNetworkTarget(target string) error {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("target must be in host:port form: %w", err)
	}
	if host == "" || host != strings.TrimSpace(host) {
		return fmt.Errorf("target host is required")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return fmt.Errorf("target port must be between 1 and 65535")
	}
	return nil
}
