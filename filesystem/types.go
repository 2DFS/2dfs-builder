package filesystem

type Allotment struct {
	Row    int    `json:"row"`
	Col    int    `json:"col"`
	Digest string `json:"digest"`
}

type Cols struct {
	Allotments    []Allotment `json:"allotments"`
	TotAllotments int         `json:"allotments_size"`
}

type TwoDFilesystem struct {
	Rows    []Cols `json:"rows"`
	TotRows int    `json:"rows_size"`
}

type Field interface {
	// AddAllotment creates the given allotment to the 2d FileSystem
	AddAllotment(allotment Allotment) Field
	// Marshal gives a marshalled filesystem as string
	Marshal() string
	// Unmarshal Given a string marshaled from TwoDFilesystem returns a Field object
	Unmarshal(string) (Field, error)
}
