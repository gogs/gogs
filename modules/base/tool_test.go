package base

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEncodeMD5(t *testing.T) {
	assert.Equal(t, "3858f62230ac3c915f300c664312c63f", EncodeMD5("foobar"))
}

func TestEncodeSha1(t *testing.T) {
	assert.Equal(t, "8843d7f92416211de9ebb963ff4ce28125932878", EncodeSha1("foobar"))
}

func TestShortSha(t *testing.T) {
	assert.Equal(t, "veryverylo", ShortSha("veryverylong"))
}

// TODO: Test DetectEncoding()

func TestBasicAuthDecode(t *testing.T) {
	_, _, err := BasicAuthDecode("?")
	assert.Equal(t, "illegal base64 data at input byte 0", err.Error())

	user, pass, err := BasicAuthDecode("Zm9vOmJhcg==")
	assert.NoError(t, err)
	assert.Equal(t, "foo", user)
	assert.Equal(t, "bar", pass)
}

func TestBasicAuthEncode(t *testing.T) {
	assert.Equal(t, "Zm9vOmJhcg==", BasicAuthEncode("foo", "bar"))
}

func TestGetRandomString(t *testing.T) {
	assert.Len(t, GetRandomString(4), 4)
}

// TODO: Test PBKDF2()
// TODO: Test VerifyTimeLimitCode()
// TODO: Test CreateTimeLimitCode()

func TestHashEmail(t *testing.T) {
	assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", HashEmail(""))
	assert.Equal(t, "353cbad9b58e69c96154ad99f92bedc7", HashEmail("gitea@example.com"))
}

// TODO: AvatarLink()
// TODO: computeTimeDiff()
// TODO: TimeSincePro()
// TODO: timeSince()
// TODO: RawTimeSince()
// TODO: TimeSince()
// TODO: logn()
// TODO: humanateBytes()

func TestFileSize(t *testing.T) {
	var size int64
	size = 512
	assert.Equal(t, "512B", FileSize(size))
	size = size * 1024
	assert.Equal(t, "512KB", FileSize(size))
	size = size * 1024
	assert.Equal(t, "512MB", FileSize(size))
	size = size * 1024
	assert.Equal(t, "512GB", FileSize(size))
	size = size * 1024
	assert.Equal(t, "512TB", FileSize(size))
	size = size * 1024
	assert.Equal(t, "512PB", FileSize(size))
	//size = size * 1024 TODO: Fix bug for EB
	//assert.Equal(t, "512EB", FileSize(size))
}

// TODO: Subtract()
// TODO: EllipsisString()
// TODO: TruncateString()
// TODO: StringsToInt64s()
// TODO: Int64sToStrings()
// TODO: Int64sToMap()
// TODO: IsLetter()
// TODO: IsTextFile()
// TODO: IsImageFile()
// TODO: IsPDFFile()
