package response

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Response struct {
	StatusCode  int
	Status      string
	Headers     http.Header
	Body        interface{}
	RawBody     []byte
	ContentType string
	Timestamp   time.Time
	Duration    time.Duration
	RequestID   string
	APIVersion  string
	Pagination  *PaginationInfo
	Error       *ErrorInfo
}

type PaginationInfo struct {
	Page       int
	PerPage    int
	Total      int
	TotalPages int
	HasNext    bool
	HasPrev    bool
	NextURL    string
	PrevURL    string
}

type ProcessingContext struct {
	Request  *http.Request
	Response *http.Response
	Options  ProcessingOptions
}

type ProcessingOptions struct {
	Format        string
	Raw           bool
	Fields        []string
	ExcludeFields []string
	Template      string
	Filter        string
	Sort          string
}

func (h *Handler) ProcessResponse(resp *http.Response, options ProcessingOptions) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	response := &Response{
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		Headers:     resp.Header,
		RawBody:     body,
		ContentType: resp.Header.Get("Content-Type"),
		Timestamp:   time.Now(),
	}

	if errHandler := NewErrorHandler(h.config.VerboseErrors); errHandler.IsError(resp) {
		response.Error = errHandler.ExtractError(resp, body)

		if h.config.Writer != nil {
			errMsg := errHandler.FormatError(response.Error)
			_, _ = fmt.Fprintln(h.config.Writer, errMsg)
		}
		return response.Error
	}

	if !options.Raw {
		detector := NewContentTypeDetector()
		format := detector.Detect(resp)

		parser := detector.GetParser(format)
		parsedData, err := parser.Parse(body)
		if err != nil {
			response.Body = string(body)
		} else {
			response.Body = parsedData
		}
	} else {
		response.Body = string(body)
	}

	formatter := h.formatter
	if options.Format != "" {
		cfg := h.config
		cfg.DefaultFormat = options.Format
		formatter = NewFormatter(cfg)
	}

	output, err := formatter.Format(response.Body)
	if err != nil {
		return fmt.Errorf("format output: %w", err)
	}

	if h.config.Writer != nil {
		_, err = h.config.Writer.Write(output)
		if err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}

	return nil
}

func (h *Handler) HandleError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read error response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	errHandler := NewErrorHandler(h.config.VerboseErrors)
	errorInfo := errHandler.ExtractError(resp, body)

	if h.config.Writer != nil {
		errMsg := errHandler.FormatError(errorInfo)
		_, _ = fmt.Fprintln(h.config.Writer, errMsg)
	}

	return errorInfo
}

func (h *Handler) ProcessStream(resp *http.Response, handler func(interface{}) error) error {
	if resp == nil {
		return fmt.Errorf("response is nil")
	}
	if handler == nil {
		return fmt.Errorf("handler is nil")
	}

	contentType := resp.Header.Get("Content-Type")
	defer func() { _ = resp.Body.Close() }()

	switch {
	case strings.Contains(contentType, "text/event-stream"):
		return h.processSSE(resp.Body, handler)
	case strings.Contains(contentType, "application/x-ndjson"),
		strings.Contains(contentType, "application/ndjson"):
		return h.processNDJSON(resp.Body, handler)
	default:
		return h.processLines(resp.Body, handler)
	}
}

func (h *Handler) processSSE(body io.Reader, handler func(interface{}) error) error {
	scanner := bufio.NewScanner(body)
	var event map[string]interface{}
	var data strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if event != nil && data.Len() > 0 {
				event["data"] = data.String()
				if err := handler(event); err != nil {
					return fmt.Errorf("handler error: %w", err)
				}
				event = nil
				data.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		field := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if event == nil {
			event = make(map[string]interface{})
		}

		switch field {
		case "event":
			event["event"] = value
		case "data":
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(value)
		case "id":
			event["id"] = value
		case "retry":
			event["retry"] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SSE stream: %w", err)
	}

	if event != nil && data.Len() > 0 {
		event["data"] = data.String()
		if err := handler(event); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	return nil
}

func (h *Handler) processNDJSON(body io.Reader, handler func(interface{}) error) error {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var data interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			return fmt.Errorf("parse NDJSON line: %w", err)
		}

		if err := handler(data); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read NDJSON stream: %w", err)
	}

	return nil
}

func (h *Handler) processLines(body io.Reader, handler func(interface{}) error) error {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if err := handler(line); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	return nil
}
