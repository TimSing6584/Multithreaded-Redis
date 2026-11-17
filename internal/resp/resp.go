package resp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var NilReply = []byte("$-1\r\n")

type Value struct {
	Type   byte
	Str    string
	Num    int
	Bulk   string
	Array  []Value
	IsNull bool
}

// The final responsibility of this file is to parse the resp request into a slice of arguments.
// The command package will then handle the arguments and return a response.

// ----Entry point----

func isRESPType(b byte) bool {
	return b == '*' || b == '+' || b == '-' || b == ':' || b == '$'
}

func ReadCommand(r *bufio.Reader) ([]string, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	// Inline command (e.g. "PING\r\n" typed into nc/telnet)
	// Useful for testing using netcat or telnet, but not part of the RESP spec.
	if !isRESPType(b) {
		rest, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return strings.Fields(string(b) + rest), nil
	}

	_ = r.UnreadByte()
	v, err := Decode(r)
	if err != nil {
		return nil, err
	}
	return extractArgs(v), nil
}

// ----RESP decoder----

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
		return decodeArray(r)
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

func decodeArray(r *bufio.Reader) (Value, error) {
	line, err := readLine(r)
	if err != nil {
		return Value{}, err
	}
	n, err := strconv.Atoi(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid array length: %q", line)
	}
	if n == -1 {
		return Value{Type: '*', IsNull: true}, nil
	}
	arr := make([]Value, n)
	for i := 0; i < n; i++ {
		val, err := Decode(r)
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