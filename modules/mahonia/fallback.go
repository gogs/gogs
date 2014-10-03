package mahonia

// FallbackDecoder combines a series of Decoders into one.
// If the first Decoder returns a status of INVALID_CHAR, the others are tried as well.
//
// Note: if the text to be decoded ends with a sequence of bytes that is not a valid character in the first charset,
// but it could be the beginning of a valid character, the FallbackDecoder will give a status of NO_ROOM instead of
// falling back to the other Decoders.
func FallbackDecoder(decoders ...Decoder) Decoder {
	return func(p []byte) (c rune, size int, status Status) {
		for _, d := range decoders {
			c, size, status = d(p)
			if status != INVALID_CHAR {
				return
			}
		}
		return 0, 1, INVALID_CHAR
	}
}
