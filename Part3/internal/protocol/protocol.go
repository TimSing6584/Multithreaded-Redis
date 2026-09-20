package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var NilReply = []byte("$-1\r\n")

// Value represents any RESP type
type Value struct {
	Type   byte    // '+', '-', ':', '$', '*'
	Str    string  // simple string / error
	Num    int     // integer
	Bulk   string  // bulk string
	Array  []Value // array
	IsNull bool    // null bulk string / null array
}

// ----Entry point (server calls this)----

// ReadCommand reads one RESP command off r and returns it as a flat []string
// of args.
func ReadCommand(r *bufio.Reader) ([]string, error) {
	// Peek, don't read: Decode reads the type byte itself, and for '$' it
	// must also consume the payload after the length line.
	b, err := r.Peek(1)
	if err != nil {
		return nil, err
	}

	switch b[0] {
	case '*', '+', '-', ':', '$':
		v, err := Decode(r)
		if err != nil {
			return nil, err
		}
		return extractArgs(v), nil
	}

	// Inline command, e.g. "SET foo bar\r\n" typed into nc/telnet.
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	return strings.Fields(line), nil
}


// ----RESP decoder/parser----

// Decode reads one complete RESP value off r.
// Used recursively by decodeArray for nested elements.
func Decode(r *bufio.Reader) (Value, error) {
	b, err := r.ReadByte()
	if err != nil {
		return Value{}, err
	}
	switch b {
	case '+':
		return decodeSimpleString(r)
	case '-':
		return decodeError(r)
	case ':':
		return decodeInteger(r)
	case '$':
		return decodeBulkString(r)
	case '*':
		line, err := readLine(r)
		if err != nil {
			return Value{}, err
		}
		return decodeArray(r, "*"+line)
	default:
		return Value{}, fmt.Errorf("unknown RESP type: %q", b)
	}
}

func decodeSimpleString(r *bufio.Reader) (Value, error) {
	line, err := readLine(r)
	if err != nil {
		return Value{}, err
	}
	return Value{Type: '+', Str: line}, nil
}

func decodeError(r *bufio.Reader) (Value, error) {
	line, err := readLine(r)
	if err != nil {
		return Value{}, err
	}
	return Value{Type: '-', Str: line}, nil
}

func decodeInteger(r *bufio.Reader) (Value, error) {
	line, err := readLine(r)
	if err != nil {
		return Value{}, err
	}
	n, err := strconv.Atoi(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid integer: %q", line)
	}
	return Value{Type: ':', Num: n}, nil
}

func decodeBulkString(r *bufio.Reader) (Value, error) {
	line, err := readLine(r)
	if err != nil {
		return Value{}, err
	}
	size, err := strconv.Atoi(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid bulk size: %q", line)
	}
	if size == -1 {
		return Value{Type: '$', IsNull: true}, nil
	}
	buf := make([]byte, size+2) // +2 for trailing \r\n
	if _, err := io.ReadFull(r, buf); err != nil {
		return Value{}, err
	}
	return Value{Type: '$', Bulk: string(buf[:size])}, nil
}

// decodeArray takes the already-consumed length line (e.g. "*3"); Decode
// reads the '*' byte and the rest of that line before calling in here.
func decodeArray(r *bufio.Reader, line string) (Value, error) {
	n, err := strconv.Atoi(line[1:]) // strip leading '*'
	if err != nil {
		return Value{}, fmt.Errorf("invalid array length: %q", line)
	}
	if n == -1 {
		return Value{Type: '*', IsNull: true}, nil
	}
	arr := make([]Value, n)
	for i := 0; i < n; i++ {
		val, err := Decode(r) // recursive
		if err != nil {
			return Value{}, err
		}
		arr[i] = val
	}
	return Value{Type: '*', Array: arr}, nil
}

// ----Helpers----

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// extractArgs flattens a decoded Value into a []string for command handling.
func extractArgs(v Value) []string {
	switch v.Type {
	case '*':
		args := make([]string, len(v.Array))
		for i, elem := range v.Array {
			switch elem.Type {
			case '$':
				args[i] = elem.Bulk
			case '+', '-':
				args[i] = elem.Str

			case ':':
				args[i] = strconv.Itoa(elem.Num)
			}
		}
		return args
	case '+', '-':
		return strings.Fields(v.Str)
	case '$':
		if v.IsNull {
			return nil
		}
		return strings.Fields(v.Bulk)
	case ':':
		return []string{strconv.Itoa(v.Num)}
	default:
		return nil
	}
}