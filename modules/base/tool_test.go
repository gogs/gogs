package base

import "testing"

func TestEncodeMD5(t *testing.T) {
	if checksum := EncodeMD5("foobar"); checksum != "3858f62230ac3c915f300c664312c63f" {
		t.Errorf("got the wrong md5sum for string foobar: %s", checksum)
	}

}

func TestEncodeSha1(t *testing.T) {
	if checksum := EncodeSha1("foobar"); checksum != "8843d7f92416211de9ebb963ff4ce28125932878" {
		t.Errorf("got the wrong sha1sum for string foobar: %s", checksum)
	}
}

func TestShortSha(t *testing.T) {
	if result := ShortSha("veryverylong"); result != "veryverylo" {
		t.Errorf("got the wrong sha1sum for string foobar: %s", result)
	}
}

// TODO: Test DetectEncoding()

func TestBasicAuthDecode(t *testing.T) {
	if _, _, err := BasicAuthDecode("?"); err.Error() != "illegal base64 data at input byte 0" {
		t.Errorf("BasicAuthDecode should fail due to illeagl data: %v", err)
	}

	user, pass, err := BasicAuthDecode("Zm9vOmJhcg==")
	if err != nil {
		t.Errorf("err should be nil but is: %v", err)
	}
	if user != "foo" {
		t.Errorf("user should be foo but is: %s", user)
	}
	if pass != "bar" {
		t.Errorf("pass should be foo but is: %s", pass)
	}
}

func TestBasicAuthEncode(t *testing.T) {
	if auth := BasicAuthEncode("foo", "bar"); auth != "Zm9vOmJhcg==" {
		t.Errorf("auth should be Zm9vOmJhcg== but is: %s", auth)
	}
}

func TestGetRandomString(t *testing.T) {
	if len(GetRandomString(4)) != 4 {
		t.Error("expected GetRandomString to be of len 4")
	}
}

// TODO: Test PBKDF2()
// TODO: Test VerifyTimeLimitCode()
// TODO: Test CreateTimeLimitCode()

func TestHashEmail(t *testing.T) {
	if hash := HashEmail("lunny@gitea.io"); hash != "1b6d0c0e124d47ded12cd7115addeb11" {
		t.Errorf("unexpected email hash: %s", hash)
	}
}

// TODO: AvatarLink()
// TODO: computeTimeDiff()
// TODO: TimeSincePro()
// TODO: timeSince()
// TODO: RawTimeSince()
// TODO: TimeSince()
// TODO: logn()
// TODO: humanateBytes()
// TODO: FileSize()
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
