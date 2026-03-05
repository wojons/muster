package response

import (
	"fmt"
	"io"
	"net/http"
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
	defer resp.Body.Close()

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
			fmt.Fprintln(h.config.Writer, errMsg)
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
	defer resp.Body.Close()

	errHandler := NewErrorHandler(h.config.VerboseErrors)
	errorInfo := errHandler.ExtractError(resp, body)

	if h.config.Writer != nil {
		errMsg := errHandler.FormatError(errorInfo)
		fmt.Fprintln(h.config.Writer, errMsg)
	}

	return errorInfo
}

func (h *Handler) ProcessStream(resp *http.Response, handler func(interface{}) error) error {
	return fmt.Errorf("streaming not implemented")
}
