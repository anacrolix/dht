package krpc

import (
	"fmt"

	"github.com/anacrolix/torrent/bencode"
)

const (
	// These are documented in BEP 5.
	ErrorCodeGenericError  = 201
	ErrorCodeServerError   = 202
	ErrorCodeProtocolError = 203
	ErrorCodeMethodUnknown = 204
	// BEP 44
	ErrorCodeMessageValueFieldTooBig       = 205
	ErrorCodeInvalidSignature              = 206
	ErrorCodeSaltFieldTooBig               = 207
	ErrorCodeCasHashMismatched             = 301
	ErrorCodeSequenceNumberLessThanCurrent = 302
)

var ErrorMethodUnknown = Error{
	Code: ErrorCodeMethodUnknown,
	Msg:  "Method Unknown",
}

// Represented as a string or list in bencode.
type Error struct {
	Code int
	Msg  string
}

var (
	_ bencode.Unmarshaler = (*Error)(nil)
	_ bencode.Marshaler   = (*Error)(nil)
	_ error               = Error{}
)

func (e *Error) UnmarshalBencode(b []byte) error {
	var v any
	if err := bencode.Unmarshal(b, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case []any:
		if len(v) < 2 {
			return fmt.Errorf("unpacking %#v: expected code and message", v)
		}
		code, ok := v[0].(int64)
		if !ok {
			return fmt.Errorf("unpacking %#v: code has type %T", v, v[0])
		}
		msg, ok := v[1].(string)
		if !ok {
			return fmt.Errorf("unpacking %#v: message has type %T", v, v[1])
		}
		e.Code = int(code)
		e.Msg = msg
	case string:
		e.Msg = v
	default:
		return fmt.Errorf("KRPC error bencode value has unexpected type: %T", v)
	}
	return nil
}

func (e Error) MarshalBencode() ([]byte, error) {
	return bencode.Marshal([]any{e.Code, e.Msg})
}

func (e Error) Error() string {
	return fmt.Sprintf("KRPC error %d: %s", e.Code, e.Msg)
}
