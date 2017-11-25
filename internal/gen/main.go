package main

import (
	"encoding/xml"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

var reservedWords = map[string]struct{}{
	"interface": {},
	"struct":    {},
}

// A documentation string. We wrap string so we can define some helper
// methods for the template's use.
type Doc string

// Return a slice of lines to be used as a Go documentation comment. The
// lines should *not* have a leading //.
func (d Doc) CommentLines() []string {
	lines := strings.Split(string(d), "\n")
	for i := range lines {
		lines[i] = strings.Trim(lines[i], " \t") + "\n"
	}

	// Skip any leading blank lines:
	i := 0
	for ; i < len(lines) && lines[i] == "\n"; i++ {
	}
	lines = lines[i:]

	// Trim off any trailing blank lines:
	for i = len(lines) - 1; i >= 0 && lines[i] == "\n"; i-- {
	}
	lines = lines[:i+1]

	for i := range lines {
		lines[i] = replaceIdentifiers(lines[i])
	}

	return lines
}

// Change wayland-style identifiers in s (e.g. wl_foo_bar) to Go style
// identifiers (e.g. FooBar):
func replaceIdentifiers(s string) string {
	words := strings.Split(s, " ")
	for i, v := range words {
		if strings.Index(v, "_") == -1 {
			// Not an identifier
			continue
		}

		words[i] = WlName(v).Exported()
	}
	return strings.Join(words, " ")
}

// Types for unmarshalling the xml file:

type Protocol struct {
	Name       WlName      `xml:"name,attr"`
	Copyright  string      `xml:"copyright"`
	Interfaces []Interface `xml:"interface"`
}

type Interface struct {
	Name        WlName    `xml:"name,attr"`
	Description Doc       `xml:"description"`
	Requests    []Request `xml:"request"`
	Events      []Event   `xml:"event"`
	Enums       []Enum    `xml:"enum"`
}

type Request struct {
	Name        WlName `xml:"name,attr"`
	Description Doc    `xml:"description"`
	Args        []Arg  `xml:"arg"`
}

type Event struct {
	Name        WlName `xml:"name,attr"`
	Description Doc    `xml:"description"`
	Args        []Arg  `xml:"arg"`
}

type Arg struct {
	Name      WlName `xml:"name,attr"`
	Type      WlType `xml:"type,attr"`
	Summary   string `xml:"summary,attr"`
	Interface WlName `xml:"interface,attr"`
}

type Enum struct {
	Name        WlName  `xml:"name,attr"`
	Description Doc     `xml:"description"`
	Bitfield    bool    `xml:"bitfield,attr"`
	Entries     []Entry `xml:"entry"`
}

type Entry struct {
	Name WlName `xml:"name,attr"`
	// We unmarshal this as a string because xml/encoding expects integers
	// to be decimal, while some of our values are hex:
	Value   string `xml:"value,attr"`
	Summary string `xml:"summary,attr"`
}

// A wrapper for wayland basic types
type WlType string

func numFdArgs(args []Arg) int {
	count := 0
	for _, arg := range args {
		if arg.Type == "fd" {
			count++
		}
	}
	return count
}

func (r *Request) NumFdArgs() int { return numFdArgs(r.Args) }
func (e *Event) NumFdArgs() int   { return numFdArgs(e.Args) }

func (t WlType) GoName() string {
	switch t {
	case "fd":
		return "int"
	case "object":
		return "ObjectId"
	case "uint":
		return "uint32"
	case "int":
		return "int32"
	case "fixed":
		return "Fixed"
	case "array":
		// TODO: the spec doesn't say anything about the element type.
		return "[]byte"
	default:
		return string(t)
	}
}

// A wrapper for wayland identifiers
type WlName string

// Split the identifier on underscores, and remove a leading "wl", if any.
func (n WlName) parts() []string {
	ret := strings.Split(string(n), "_")
	if ret[0] == "wl" {
		ret = ret[1:]
	}
	return ret
}

// Convert each element in parts to title case.
func titleCase(parts []string) {
	for i, part := range parts {
		parts[i] = strings.Title(part)
	}
}

// Convert the identifier to an exported idiomatic go variable name.
func (n WlName) Exported() string {
	parts := n.parts()
	titleCase(parts)
	return strings.Join(parts, "")
}

// Convert the identifier to a private/local idiomatic go variable name.
func (n WlName) Local() string {
	parts := n.parts()
	titleCase(parts[1:])
	ret := strings.Join(parts, "")
	_, ok := reservedWords[ret]
	if ok {
		ret += "_"
	}
	return ret
}

// Helper for simple error handling
func chkfatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	tpls := template.Must(template.ParseGlob("./internal/gen/templates/*"))
	proto := Protocol{}
	buf, err := ioutil.ReadFile("wayland.xml")
	chkfatal(err)
	err = xml.Unmarshal(buf, &proto)
	chkfatal(err)
	file, err := os.Create("gen.go")
	chkfatal(err)
	defer file.Close()
	chkfatal(tpls.ExecuteTemplate(file, "protocol", proto))
	chkfatal(exec.Command("gofmt", "-s", "-w", "gen.go").Run())
}
