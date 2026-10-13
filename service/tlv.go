package service

import "fmt"

// TLV represents a single BER-TLV node.
type TLV struct {
	Tag      uint32
	Value    []byte
	Children []TLV
}

// ParseTLV parses a byte slice as a sequence of BER-TLV structures.
func ParseTLV(data []byte) ([]TLV, error) {
	var result []TLV
	i := 0
	for i < len(data) {
		if i >= len(data) {
			break
		}

		// Parse tag
		tag := uint32(data[i])
		i++
		isConstructed := (tag & 0x20) != 0
		if tag&0x1F == 0x1F {
			// Multi-byte tag
			for i < len(data) {
				b := data[i]
				i++
				tag = (tag << 8) | uint32(b)
				if b&0x80 == 0 {
					break
				}
			}
		}

		// Parse length
		if i >= len(data) {
			return nil, fmt.Errorf("TLV: truncated at length byte for tag %X", tag)
		}
		length := int(data[i])
		i++
		if length == 0x80 {
			return nil, fmt.Errorf("TLV: indefinite length not supported")
		}
		if length > 0x80 {
			numBytes := length & 0x7F
			length = 0
			for j := 0; j < numBytes; j++ {
				if i >= len(data) {
					return nil, fmt.Errorf("TLV: truncated in long length")
				}
				length = (length << 8) | int(data[i])
				i++
			}
		}

		if i+length > len(data) {
			return nil, fmt.Errorf("TLV: value length %d exceeds data at tag %X", length, tag)
		}
		value := data[i : i+length]
		i += length

		node := TLV{Tag: tag, Value: value}
		if isConstructed {
			children, err := ParseTLV(value)
			if err != nil {
				return nil, err
			}
			node.Children = children
		}
		result = append(result, node)
	}
	return result, nil
}

// FindTag performs a depth-first search for the first TLV with the given tag.
func FindTag(tlvs []TLV, tag uint32) *TLV {
	for i := range tlvs {
		if tlvs[i].Tag == tag {
			return &tlvs[i]
		}
		if found := FindTag(tlvs[i].Children, tag); found != nil {
			return found
		}
	}
	return nil
}

// FindAllTags returns all TLVs at the top level of the list with the given tag (no recursion).
func FindAllTags(tlvs []TLV, tag uint32) []TLV {
	var result []TLV
	for _, t := range tlvs {
		if t.Tag == tag {
			result = append(result, t)
		}
	}
	return result
}
