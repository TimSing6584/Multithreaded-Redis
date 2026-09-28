package resp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Represenation of a nil value with RESP format.
var NilReply = []byte("$-1\r\n")

// --------Encoder--------

type Value interface{}

type SimpleString string
type Error string
type Integer int
type BulkString string
type Array []Value

func EncodeReply(v Value) []byte {
	switch x := v.(type) {
	case SimpleString:
		return []byte("+" + string(x) + "\r\n")
	case Error:
		return []byte("-" + string(x) + "\r\n")
	case Integer:
		return []byte(fmt.Sprintf(":%d\r\n", int(x)))
	case BulkString:
		return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(x), x))
	case Array:
		buf := []byte(fmt.Sprintf("*%d\r\n", len(x)))
		for _, elem := range x {
				buf = append(buf, EncodeReply(elem)...)
		}
		return buf

	default:
		return []byte("-ERR internal: unencodable reply\r\n")
	}
}



// --------Helpers--------


// IncompleteError: this happens when the client sends a partial command, and we need to wait for more data before we can parse it.
var IncompleteError = errors.New("protocol error: incomplete command")

// In RESP, a command is represented as an array of bulk strings with the format "Command Name" + "A set of arguments"
// For example, the command "SET key value" would be represented as: *3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n
type Command struct {
	Name string
	Args []string
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// --------Entry point (called from server) --------
func ParseCommand(r *bufio.Reader) (Command, error) {
	line, err := readLine(r)
	if err != nil {
		return Command{}, IncompleteError
	}
	if len(line) == 0 {
		return Command{}, nil
	}

	if line[0] != '*' {
		// Inline command (e.g. "PING\r\n" typed into nc/telnet)
		// Useful for testing using netcat or telnet, but not part of the RESP spec.
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return Command{}, nil
		}
		return Command{Name: fields[0], Args: fields[1:]}, nil
	}

	return decodeCommandArray(r, line)
}

// decodeCommandArray decodes the elements of a RESP array - the "*N" length
// line has already been read into `line` by ParseCommand - into a Command.
// A command is always an array of bulk strings: the command name followed
// by its arguments, so each element is read with decodeBulkString.
func decodeCommandArray(r *bufio.Reader, line string) (Command, error) {
	n, err := strconv.Atoi(line[1:]) // strip the leading '*' to get the number of elements in the array
	if err != nil {
		return Command{}, fmt.Errorf("invalid array length: %q", line)
	}
	if n <= 0 {
		// "*0\r\n" (empty array) or "*-1\r\n" (null array) - no command.
		return Command{}, nil
	}

	args := make([]string, n)
	for i := 0; i < n; i++ {
		s, err := decodeBulkString(r)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				// The array header arrived, but an element didn't fully -
				// that's still an incomplete command, not a malformed one.
				return Command{}, IncompleteError
			}
			return Command{}, err
		}
		args[i] = s
	}
	return Command{Name: args[0], Args: args[1:]}, nil
}

// decodeBulkString reads one RESP bulk string element: "$<size>\r\n<data>\r\n".
// Real clients always send command elements this way, so it's the only
// element type ParseCommand needs to understand.
func decodeBulkString(r *bufio.Reader) (string, error) {
	typeByte, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if typeByte != '$' {
		return "", fmt.Errorf("expected bulk string ('$'), got %q", typeByte)
	}

	line, err := readLine(r)
	if err != nil {
		return "", err
	}
	size, err := strconv.Atoi(line)
	if err != nil {
		return "", fmt.Errorf("invalid bulk size: %q", line)
	}
	if size < 0 {
		// Null bulk string ("$-1\r\n") - treat as an empty argument.
		return "", nil
	}

	buf := make([]byte, size+2) // +2 for the trailing \r\n
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf[:size]), nil
}
