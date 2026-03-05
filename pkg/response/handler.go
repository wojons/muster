package response

import (
	"io"
)

type Config struct {
	DefaultFormat    string
	SupportedFormats []string
	ContentTypeMap   map[string]string
	DefaultType      string
	ErrorFormat      string
	VerboseErrors    bool
	IncludeHeaders   bool
	RawOutput        bool
	Pagination       PaginationConfig
	Filtering        FilterConfig
	Writer           io.Writer
}

type PaginationConfig struct {
	Enabled     bool
	LimitParam  string
	OffsetParam string
	PageParam   string
	SizeParam   string
	TotalHeader string
	NextHeader  string
}

type FilterConfig struct {
	Enabled       bool
	Fields        []string
	ExcludeFields []string
}

type Handler struct {
	config    Config
	parser    ContentParser
	formatter Formatter
}

func NewHandler(config Config) *Handler {
	return &Handler{
		config:    config,
		parser:    NewContentParser(config),
		formatter: NewFormatter(config),
	}
}

func (h *Handler) SetWriter(w io.Writer) {
	h.config.Writer = w
}
