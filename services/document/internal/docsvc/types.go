package docsvc

type documentInput struct {
	SourceURI  string
	ContentRef string
	Content    []byte
	Filename   string
	MIMEType   string
	Metadata   map[string]string
}

type sourceDocument struct {
	SourceURI string
	Path      string
	Content   []byte
	Filename  string
	MIMEType  string
	Metadata  map[string]string
}

type detection struct {
	MIMEType   string
	Extension  string
	Family     string
	Confidence float32
	Metadata   map[string]string
}

type parsedDocument struct {
	DocumentID    string
	SourceHash    string
	MIMEType      string
	Family        string
	PageCount     int32
	TextAvailable bool
	Metadata      map[string]string
	Pages         []textPage
	Tables        []table
	Images        []image
	Layouts       []layoutPage
}

type textPage struct {
	PageNumber  int32
	Text        string
	StartOffset int32
	EndOffset   int32
}

type layoutPage struct {
	PageNumber int32
	Width      float32
	Height     float32
	Blocks     []layoutBlock
}

type layoutBlock struct {
	Kind string
	Text string
	Box  box
}

type box struct {
	X      float32
	Y      float32
	Width  float32
	Height float32
}

type table struct {
	PageNumber int32
	Title      string
	Headers    []string
	Rows       []tableRow
	Box        box
}

type tableRow struct {
	Cells []string
}

type image struct {
	PageNumber int32
	ImageRef   string
	MIMEType   string
	Box        box
	Metadata   map[string]string
	Content    []byte
	SourceURI  string
	SourceHash string
}
