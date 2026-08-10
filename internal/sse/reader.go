package sse

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

type Reader struct {
	src *bufio.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{src: bufio.NewReaderSize(r, 64*1024)}
}

func (r *Reader) Next() ([]byte, error) {
	var data []string
	for {
		line, err := r.src.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if payload, ok := cutData(strings.TrimRight(line, "\r\n")); ok {
					data = append(data, payload)
				}
				if len(data) > 0 {
					return []byte(strings.Join(data, "\n")), nil
				}
			}
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if len(data) > 0 {
				return []byte(strings.Join(data, "\n")), nil
			}

		case strings.HasPrefix(line, ":"):

		default:
			if payload, ok := cutData(line); ok {
				data = append(data, payload)
			}

		}
	}
}

func cutData(line string) (string, bool) {
	payload, ok := strings.CutPrefix(line, "data:")
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(payload, " "), true
}
