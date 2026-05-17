package docsvc

import "errors"

var (
	errEmptyInput        = errors.New("document input is empty")
	errUnsupportedType   = errors.New("document type is unsupported")
	errPDFBackendMissing = errors.New("pdf text extraction backend is unavailable")
	errPDFParseFailed    = errors.New("pdf text extraction failed")
	errContentRefOnly    = errors.New("content_ref resolution is not configured")
	errOCRBackendMissing = errors.New("ocr backend is not configured")
)
