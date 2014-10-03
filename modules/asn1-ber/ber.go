package ber

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
)

type Packet struct {
	ClassType   uint8
	TagType     uint8
	Tag         uint8
	Value       interface{}
	ByteValue   []byte
	Data        *bytes.Buffer
	Children    []*Packet
	Description string
}

const (
	TagEOC              = 0x00
	TagBoolean          = 0x01
	TagInteger          = 0x02
	TagBitString        = 0x03
	TagOctetString      = 0x04
	TagNULL             = 0x05
	TagObjectIdentifier = 0x06
	TagObjectDescriptor = 0x07
	TagExternal         = 0x08
	TagRealFloat        = 0x09
	TagEnumerated       = 0x0a
	TagEmbeddedPDV      = 0x0b
	TagUTF8String       = 0x0c
	TagRelativeOID      = 0x0d
	TagSequence         = 0x10
	TagSet              = 0x11
	TagNumericString    = 0x12
	TagPrintableString  = 0x13
	TagT61String        = 0x14
	TagVideotexString   = 0x15
	TagIA5String        = 0x16
	TagUTCTime          = 0x17
	TagGeneralizedTime  = 0x18
	TagGraphicString    = 0x19
	TagVisibleString    = 0x1a
	TagGeneralString    = 0x1b
	TagUniversalString  = 0x1c
	TagCharacterString  = 0x1d
	TagBMPString        = 0x1e
	TagBitmask          = 0x1f // xxx11111b
)

var TagMap = map[uint8]string{
	TagEOC:              "EOC (End-of-Content)",
	TagBoolean:          "Boolean",
	TagInteger:          "Integer",
	TagBitString:        "Bit String",
	TagOctetString:      "Octet String",
	TagNULL:             "NULL",
	TagObjectIdentifier: "Object Identifier",
	TagObjectDescriptor: "Object Descriptor",
	TagExternal:         "External",
	TagRealFloat:        "Real (float)",
	TagEnumerated:       "Enumerated",
	TagEmbeddedPDV:      "Embedded PDV",
	TagUTF8String:       "UTF8 String",
	TagRelativeOID:      "Relative-OID",
	TagSequence:         "Sequence and Sequence of",
	TagSet:              "Set and Set OF",
	TagNumericString:    "Numeric String",
	TagPrintableString:  "Printable String",
	TagT61String:        "T61 String",
	TagVideotexString:   "Videotex String",
	TagIA5String:        "IA5 String",
	TagUTCTime:          "UTC Time",
	TagGeneralizedTime:  "Generalized Time",
	TagGraphicString:    "Graphic String",
	TagVisibleString:    "Visible String",
	TagGeneralString:    "General String",
	TagUniversalString:  "Universal String",
	TagCharacterString:  "Character String",
	TagBMPString:        "BMP String",
}

const (
	ClassUniversal   = 0   // 00xxxxxxb
	ClassApplication = 64  // 01xxxxxxb
	ClassContext     = 128 // 10xxxxxxb
	ClassPrivate     = 192 // 11xxxxxxb
	ClassBitmask     = 192 // 11xxxxxxb
)

var ClassMap = map[uint8]string{
	ClassUniversal:   "Universal",
	ClassApplication: "Application",
	ClassContext:     "Context",
	ClassPrivate:     "Private",
}

const (
	TypePrimitive   = 0  // xx0xxxxxb
	TypeConstructed = 32 // xx1xxxxxb
	TypeBitmask     = 32 // xx1xxxxxb
)

var TypeMap = map[uint8]string{
	TypePrimitive:   "Primative",
	TypeConstructed: "Constructed",
}

var Debug bool = false

func PrintBytes(buf []byte, indent string) {
	data_lines := make([]string, (len(buf)/30)+1)
	num_lines := make([]string, (len(buf)/30)+1)

	for i, b := range buf {
		data_lines[i/30] += fmt.Sprintf("%02x ", b)
		num_lines[i/30] += fmt.Sprintf("%02d ", (i+1)%100)
	}

	for i := 0; i < len(data_lines); i++ {
		fmt.Print(indent + data_lines[i] + "\n")
		fmt.Print(indent + num_lines[i] + "\n\n")
	}
}

func PrintPacket(p *Packet) {
	printPacket(p, 0, false)
}

func printPacket(p *Packet, indent int, printBytes bool) {
	indent_str := ""

	for len(indent_str) != indent {
		indent_str += " "
	}

	class_str := ClassMap[p.ClassType]

	tagtype_str := TypeMap[p.TagType]

	tag_str := fmt.Sprintf("0x%02X", p.Tag)

	if p.ClassType == ClassUniversal {
		tag_str = TagMap[p.Tag]
	}

	value := fmt.Sprint(p.Value)
	description := ""

	if p.Description != "" {
		description = p.Description + ": "
	}

	fmt.Printf("%s%s(%s, %s, %s) Len=%d %q\n", indent_str, description, class_str, tagtype_str, tag_str, p.Data.Len(), value)

	if printBytes {
		PrintBytes(p.Bytes(), indent_str)
	}

	for _, child := range p.Children {
		printPacket(child, indent+1, printBytes)
	}
}

func resizeBuffer(in []byte, new_size uint64) (out []byte) {
	out = make([]byte, new_size)

	copy(out, in)

	return
}

func readBytes(reader io.Reader, buf []byte) error {
	idx := 0
	buflen := len(buf)

	for idx < buflen {
		n, err := reader.Read(buf[idx:])
		if err != nil {
			return err
		}
		idx += n
	}

	return nil
}

func ReadPacket(reader io.Reader) (*Packet, error) {
	buf := make([]byte, 2)

	err := readBytes(reader, buf)

	if err != nil {
		return nil, err
	}

	idx := uint64(2)
	datalen := uint64(buf[1])

	if Debug {
		fmt.Printf("Read: datalen = %d len(buf) = %d ", datalen, len(buf))

		for _, b := range buf {
			fmt.Printf("%02X ", b)
		}

		fmt.Printf("\n")
	}

	if datalen&128 != 0 {
		a := datalen - 128

		idx += a
		buf = resizeBuffer(buf, 2+a)

		err := readBytes(reader, buf[2:])

		if err != nil {
			return nil, err
		}

		datalen = DecodeInteger(buf[2 : 2+a])

		if Debug {
			fmt.Printf("Read: a = %d  idx = %d  datalen = %d  len(buf) = %d", a, idx, datalen, len(buf))

			for _, b := range buf {
				fmt.Printf("%02X ", b)
			}

			fmt.Printf("\n")
		}
	}

	buf = resizeBuffer(buf, idx+datalen)
	err = readBytes(reader, buf[idx:])

	if err != nil {
		return nil, err
	}

	if Debug {
		fmt.Printf("Read: len( buf ) = %d  idx=%d datalen=%d idx+datalen=%d\n", len(buf), idx, datalen, idx+datalen)

		for _, b := range buf {
			fmt.Printf("%02X ", b)
		}
	}

	p := DecodePacket(buf)

	return p, nil
}

func DecodeString(data []byte) (ret string) {
	for _, c := range data {
		ret += fmt.Sprintf("%c", c)
	}

	return
}

func DecodeInteger(data []byte) (ret uint64) {
	for _, i := range data {
		ret = ret * 256
		ret = ret + uint64(i)
	}

	return
}

func EncodeInteger(val uint64) []byte {
	var out bytes.Buffer

	found := false

	shift := uint(56)

	mask := uint64(0xFF00000000000000)

	for mask > 0 {
		if !found && (val&mask != 0) {
			found = true
		}

		if found || (shift == 0) {
			out.Write([]byte{byte((val & mask) >> shift)})
		}

		shift -= 8
		mask = mask >> 8
	}

	return out.Bytes()
}

func DecodePacket(data []byte) *Packet {
	p, _ := decodePacket(data)

	return p
}

func decodePacket(data []byte) (*Packet, []byte) {
	if Debug {
		fmt.Printf("decodePacket: enter %d\n", len(data))
	}

	p := new(Packet)

	p.ClassType = data[0] & ClassBitmask
	p.TagType = data[0] & TypeBitmask
	p.Tag = data[0] & TagBitmask

	datalen := DecodeInteger(data[1:2])
	datapos := uint64(2)

	if datalen&128 != 0 {
		datalen -= 128
		datapos += datalen
		datalen = DecodeInteger(data[2 : 2+datalen])
	}

	p.Data = new(bytes.Buffer)

	p.Children = make([]*Packet, 0, 2)

	p.Value = nil

	value_data := data[datapos : datapos+datalen]

	if p.TagType == TypeConstructed {
		for len(value_data) != 0 {
			var child *Packet

			child, value_data = decodePacket(value_data)
			p.AppendChild(child)
		}
	} else if p.ClassType == ClassUniversal {
		p.Data.Write(data[datapos : datapos+datalen])
		p.ByteValue = value_data

		switch p.Tag {
		case TagEOC:
		case TagBoolean:
			val := DecodeInteger(value_data)

			p.Value = val != 0
		case TagInteger:
			p.Value = DecodeInteger(value_data)
		case TagBitString:
		case TagOctetString:
			p.Value = DecodeString(value_data)
		case TagNULL:
		case TagObjectIdentifier:
		case TagObjectDescriptor:
		case TagExternal:
		case TagRealFloat:
		case TagEnumerated:
			p.Value = DecodeInteger(value_data)
		case TagEmbeddedPDV:
		case TagUTF8String:
		case TagRelativeOID:
		case TagSequence:
		case TagSet:
		case TagNumericString:
		case TagPrintableString:
			p.Value = DecodeString(value_data)
		case TagT61String:
		case TagVideotexString:
		case TagIA5String:
		case TagUTCTime:
		case TagGeneralizedTime:
		case TagGraphicString:
		case TagVisibleString:
		case TagGeneralString:
		case TagUniversalString:
		case TagCharacterString:
		case TagBMPString:
		}
	} else {
		p.Data.Write(data[datapos : datapos+datalen])
	}

	return p, data[datapos+datalen:]
}

func (p *Packet) DataLength() uint64 {
	return uint64(p.Data.Len())
}

func (p *Packet) Bytes() []byte {
	var out bytes.Buffer

	out.Write([]byte{p.ClassType | p.TagType | p.Tag})
	packet_length := EncodeInteger(p.DataLength())

	if p.DataLength() > 127 || len(packet_length) > 1 {
		out.Write([]byte{byte(len(packet_length) | 128)})
		out.Write(packet_length)
	} else {
		out.Write(packet_length)
	}

	out.Write(p.Data.Bytes())

	return out.Bytes()
}

func (p *Packet) AppendChild(child *Packet) {
	p.Data.Write(child.Bytes())

	if len(p.Children) == cap(p.Children) {
		newChildren := make([]*Packet, cap(p.Children)*2)

		copy(newChildren, p.Children)
		p.Children = newChildren[0:len(p.Children)]
	}

	p.Children = p.Children[0 : len(p.Children)+1]
	p.Children[len(p.Children)-1] = child
}

func Encode(ClassType, TagType, Tag uint8, Value interface{}, Description string) *Packet {
	p := new(Packet)

	p.ClassType = ClassType
	p.TagType = TagType
	p.Tag = Tag
	p.Data = new(bytes.Buffer)

	p.Children = make([]*Packet, 0, 2)

	p.Value = Value
	p.Description = Description

	if Value != nil {
		v := reflect.ValueOf(Value)

		if ClassType == ClassUniversal {
			switch Tag {
			case TagOctetString:
				sv, ok := v.Interface().(string)

				if ok {
					p.Data.Write([]byte(sv))
				}
			}
		}
	}

	return p
}

func NewSequence(Description string) *Packet {
	return Encode(ClassUniversal, TypePrimitive, TagSequence, nil, Description)
}

func NewBoolean(ClassType, TagType, Tag uint8, Value bool, Description string) *Packet {
	intValue := 0

	if Value {
		intValue = 1
	}

	p := Encode(ClassType, TagType, Tag, nil, Description)

	p.Value = Value
	p.Data.Write(EncodeInteger(uint64(intValue)))

	return p
}

func NewInteger(ClassType, TagType, Tag uint8, Value uint64, Description string) *Packet {
	p := Encode(ClassType, TagType, Tag, nil, Description)

	p.Value = Value
	p.Data.Write(EncodeInteger(Value))

	return p
}

func NewString(ClassType, TagType, Tag uint8, Value, Description string) *Packet {
	p := Encode(ClassType, TagType, Tag, nil, Description)

	p.Value = Value
	p.Data.Write([]byte(Value))

	return p
}
