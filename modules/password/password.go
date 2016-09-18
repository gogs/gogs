package password

import (
  "fmt"
  "errors"
  "hash"
  "strings"
  "bytes"
  "strconv"
  "crypto/sha256"
  "crypto/sha512"
  "encoding/base64"
  "golang.org/x/crypto/pbkdf2"
)

var SUPPORTED_HASHES = map[string]func() hash.Hash{
  "PBKDF2-HMAC-SHA256": sha256.New,
  "PBKDF2-HMAC-SHA512": sha512.New,
}

type Password struct {
  iterations int
  keyLength int
  hashAlgorithm func() hash.Hash
  checksum []byte
  salt string
}

func Verify(clear string, passwd string) (bool, error) {

  password, err := Identify(passwd)
  if(err != nil) {
    return false, err
  }

  return VerifyManual(clear, password.checksum, password.salt, password.iterations, password.keyLength, password.hashAlgorithm)

}

func VerifyBackwardsCompatible(clear string, checksum string, salt string) bool {

  newChecksum := encrypt(clear, salt, 10000, 50, sha256.New)
  compatibleChecksum := fmt.Sprintf("%x", newChecksum)

  return checksum == compatibleChecksum

}

func VerifyManual(clear string, checksum []byte, salt string, iterations int, keyLength int, hashAlgorithm func() hash.Hash) (bool, error) {
  newChecksum := encrypt(clear, salt, iterations, keyLength, hashAlgorithm);
  result := bytes.Equal(checksum, newChecksum)
  return result, nil
}

func Identify(passwd string) (Password, error) {

  var result Password

  for digest, algo := range SUPPORTED_HASHES {

    scheme := "{" + digest + "}";
    if strings.HasPrefix(strings.ToUpper(passwd), scheme) {

      result.hashAlgorithm = algo

      passwordParameters := strings.Split(passwd, "$")
      if(len(passwordParameters) != 4) {
        return result, errors.New("password identification failed: invalid number of parameters")
      }

      // parse iterations
      iterations, err := strconv.Atoi(passwordParameters[1])
      if (err != nil) || (iterations <= 0) {
        return result, errors.New("password identification failed: invalid amount of iterations")
      }
      result.iterations = iterations

      // parse salt
      salt, err := base64.StdEncoding.DecodeString(passwordParameters[2])
      if err != nil {
        return result, errors.New("password identification failed: decoding of base64 salt failed")
      }
      result.salt = string(salt[:])

      // parse checksum
      checksum, err := base64.StdEncoding.DecodeString(passwordParameters[3])
      if err != nil {
        return result, errors.New("password identification failed: decoding of base64 checksum failed")
      }
      result.checksum = checksum

      // calculate keyLength from checksum length
      result.keyLength = len(result.checksum)

      return result, nil

    }

  }

  return result, errors.New("password identification failed: no supported algorithm found")

}

func Hash(clear string, salt string, iterations int, keyLength int, digest string) (string, error) {

  hashAlgorithm, err := GetHashAlgorithm(digest);
  if(err != nil) {
    // ToDo: Exit with real error
    return "", err
  }

  output := []string{
    "{" + digest + "}",
    strconv.Itoa(iterations),
    base64.StdEncoding.EncodeToString([]byte(salt)),
    base64.StdEncoding.EncodeToString(encrypt(clear, salt, iterations, keyLength, hashAlgorithm)),
  }

  return strings.Join(output, "$"), nil

}

func GetHashAlgorithm(digest string) (func() hash.Hash, error) {
  if hashAlgorithm, ok := SUPPORTED_HASHES[digest]; ok {
    return hashAlgorithm, nil
  } else {
    return nil, errors.New("Cannot calculate hash with unknown digest: " + digest)
  }
}

func encrypt(clear string, salt string, iterations int, keyLength int, hashAlgorithm func() hash.Hash) []byte {
  out := pbkdf2.Key([]byte(clear), []byte(salt), iterations, keyLength, hashAlgorithm)
  return out;
}
