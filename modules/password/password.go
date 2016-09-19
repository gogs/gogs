package password

import (
  "fmt"
  "errors"
  "hash"
  "strings"
  "bytes"
  "strconv"
  "crypto/rand"
  "crypto/sha256"
  "crypto/sha512"
  "encoding/base64"
  "golang.org/x/crypto/pbkdf2"
  "golang.org/x/crypto/bcrypt"
)

var SUPPORTED_HASHES = []string{
  "PBKDF2-HMAC-SHA256",
  "PBKDF2-HMAC-SHA512",
  "BCRYPT",
}

const (
  DEFAULT_DIGEST string = "BCRYPT"
  DEFAULT_PBKDF2_ITERATIONS int = 10000
  DEFAULT_PBKDF2_SALT_LENGTH = 64 // byte
  DEFAULT_PBKDF2_OUTPUT_LENGTH = 64
  DEFAULT_BCRYPT_COST = 10
)

type Password struct {
  digest string
  checksum []byte
  // pkbdf2
  iterations int
  keyLength int
  salt []byte
  hashAlgorithm func() hash.Hash
}

func Verify(clear string, passwd string) (bool, error) {

  hashInfo, err := Identify(passwd)
  if(err != nil) {
    return false, err
  }

  verified := false;

  switch getDigestMethod(hashInfo.digest) {
  case "PBKDF2":
    verified = VerifyPBKDF2(clear, hashInfo.checksum, hashInfo.salt, hashInfo.iterations, hashInfo.keyLength, hashInfo.hashAlgorithm)
  case "BCRYPT":
    verified = VerifyBCRYPT(clear, hashInfo.checksum)
  }

  return (verified == true), nil

}

func VerifyBackwardsCompatible(clear string, checksum string, salt []byte) bool {
  newChecksum := encrypt(clear, salt, 10000, 50, sha256.New)
  compatibleChecksum := fmt.Sprintf("%x", newChecksum)
  return checksum == compatibleChecksum
}

func VerifyPBKDF2(clear string, checksum []byte, salt []byte, iterations int, keyLength int, hashAlgorithm func() hash.Hash) bool {
  newChecksum := encrypt(clear, salt, iterations, keyLength, hashAlgorithm);
  result := bytes.Equal(checksum, newChecksum)
  return result
}

func VerifyBCRYPT(clear string, checksum []byte) bool {
  err := bcrypt.CompareHashAndPassword(checksum, []byte(clear));
  return (err == nil)
}

func Identify(passwd string) (Password, error) {

  var result Password

  for _, digest := range SUPPORTED_HASHES {

    scheme := "{" + digest + "}";
    if strings.HasPrefix(strings.ToUpper(passwd), scheme) {

      result.digest = digest

      if strings.HasPrefix(digest, "PBKDF2-") {

        hashAlgorithm, err := getHashAlgorithm(digest)
        if err != nil {
          return result, err
        }
        result.hashAlgorithm = hashAlgorithm

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
        result.salt = salt

        // parse checksum
        checksum, err := base64.StdEncoding.DecodeString(passwordParameters[3])
        if err != nil {
          return result, errors.New("password identification failed: decoding of base64 checksum failed")
        }
        result.checksum = checksum

        // calculate keyLength from checksum length
        result.keyLength = len(result.checksum)
        return result, nil

      } else if (digest == "BCRYPT") {

        result.checksum = []byte(passwd[8:])
        return result, nil

      }

    }

  }

  return result, errors.New("password identification failed: no supported algorithm found")

}

func Hash(clear string) (string, error) {

  switch getDigestMethod(DEFAULT_DIGEST) {
  case "BCRYPT":
    return HashBCRYPT(clear, DEFAULT_BCRYPT_COST)
  case "PBKDF2":
    return HashPBKDF2(clear, generateSalt(), DEFAULT_PBKDF2_ITERATIONS, DEFAULT_PBKDF2_OUTPUT_LENGTH, DEFAULT_DIGEST)
  }

  return "", errors.New("invalid default hash digest: unknown hash method")

}

func HashBCRYPT(clear string, cost int) (string, error) {
  encrypted, err := bcrypt.GenerateFromPassword([]byte(clear), cost)
  if(err != nil) {
    return "", err
  }
  return "{BCRYPT}" + string(encrypted[:]), nil
}

func HashPBKDF2(clear string, salt []byte, iterations int, keyLength int, digest string) (string, error) {
  hashAlgorithm, err := getHashAlgorithm(digest)
  if(err != nil) {
    return "", errors.New("invalid default hash digest: unknown hash algorithm")
  }
  output := []string{
    "{" + digest + "}",
    strconv.Itoa(iterations),
    base64.StdEncoding.EncodeToString(salt),
    base64.StdEncoding.EncodeToString(encrypt(clear, salt, iterations, keyLength, hashAlgorithm)),
  }
  return strings.Join(output, "$"), nil
}

func getHashAlgorithm(digest string) (func() hash.Hash, error) {
  
  switch strings.ToUpper(digest) {
  case "PBKDF2-HMAC-SHA256":
    return sha256.New, nil
  case "PBKDF2-HMAC-SHA512":
    return sha512.New, nil
  }

  return nil, errors.New("cannot getHashAlgorithm from unknown digest: " + digest)

}

func encrypt(clear string, salt []byte, iterations int, keyLength int, hashAlgorithm func() hash.Hash) []byte {
  out := pbkdf2.Key([]byte(clear), salt, iterations, keyLength, hashAlgorithm)
  return out;
}

func generateSalt() []byte {
  n := DEFAULT_PBKDF2_SALT_LENGTH
  b := make([]byte, n)
  _, err := rand.Read(b)
  if err != nil {
    // ToDo: Panic, RNG is failing
  }
  return b
}

func getDigestMethod(digest string) string {
  firstSeparatorIndex := strings.Index(digest, "-")
  if firstSeparatorIndex < 1 {
    return digest
  }
  return digest[0:firstSeparatorIndex]
}
