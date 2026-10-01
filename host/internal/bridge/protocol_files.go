package bridge

// Attachment and file payloads (protocol spec §6): M3.

// AttachBegin starts or resumes an upload from the phone.
type AttachBegin struct {
	ID     string `json:"id"` // chosen by the app: 32 lower-case hex digits
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// AttachReady tells the app where to go on: the bytes the host already has
// of an upload with the same id and hash, or 0.
type AttachReady struct {
	ID     string `json:"id"`
	Offset int64  `json:"offset"`
}

// AttachEnd says every byte of an upload was sent.
type AttachEnd struct {
	ID string `json:"id"`
}

// AttachDone is a finished upload, kept in inbox/.
type AttachDone struct {
	Attachment Attachment `json:"attachment"`
}

// FileGet asks for an attachment's bytes from Offset on.
type FileGet struct {
	ID     string `json:"id"`
	Offset int64  `json:"offset"`
}

// FileEnd follows the last binary frame of a download.
type FileEnd struct {
	ID   string `json:"id"`
	Size int64  `json:"size"`
}

// FileCancel stops a download in progress.
type FileCancel struct {
	ID string `json:"id"`
}

// CaptureReady is a screenshot the host took for capture.request, kept in inbox/.
type CaptureReady struct {
	Attachment Attachment `json:"attachment"`
}
