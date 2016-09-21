package password

import (
  "log"
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
  DEFAULT_ALGORITHM string = "BCRYPT"
  DEFAULT_PBKDF2_ITERATIONS int = 10000
  DEFAULT_PBKDF2_SALT_LENGTH = 64 // byte
  DEFAULT_PBKDF2_OUTPUT_LENGTH = 64
  DEFAULT_BCRYPT_COST = 10
)

type Password struct {
  algorithm string
  checksum []byte
  // pkbdf2
  iterations int
  keyLength int
  salt []byte
  hashAlgorithm func() hash.Hash
}

var ENABLED_ALGORITHM string = DEFAULT_ALGORITHM

func UseHashAlgorithm(value string) {
  if(value == "") {
    return
  }
  for _, supportedAlgorithm := range SUPPORTED_HASHES {
    if(value == supportedAlgorithm) {
      ENABLED_ALGORITHM = value
      return
    }
  }
  log.Fatalf("invalid password hash algorithm: %s", value)
}

func Verify(clear string, passwd string) (bool, error) {

  hashInfo, err := Identify(passwd)
  if(err != nil) {
    return false, err
  }

  verified := false;

  switch getMethod(hashInfo.algorithm) {
  case "PBKDF2":
    verified = VerifyPBKDF2(clear, hashInfo.checksum, hashInfo.salt, hashInfo.iterations, hashInfo.keyLength, hashInfo.hashAlgorithm)
  case "BCRYPT":
    verified = VerifyBCRYPT(clear, hashInfo.checksum)
  }

  if(hashInfo.algorithm != ENABLED_ALGORITHM) {
    return (verified == true), errors.New("password hash algorithm is deprecated")
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

  for _, algorithm := range SUPPORTED_HASHES {

    scheme := "{" + algorithm + "}";
    if strings.HasPrefix(strings.ToUpper(passwd), scheme) {

      result.algorithm = algorithm

      if strings.HasPrefix(algorithm, "PBKDF2-") {

        hashAlgorithm, err := getHashAlgorithm(algorithm)
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

      } else if (algorithm == "BCRYPT") {

        result.checksum = []byte(passwd[8:])
        return result, nil

      }

    }

  }

  return result, errors.New("password identification failed: no supported algorithm found")

}

func Hash(clear string) (string, error) {

  switch getMethod(ENABLED_ALGORITHM) {
  case "BCRYPT":
    return HashBCRYPT(clear, DEFAULT_BCRYPT_COST)
  case "PBKDF2":
    return HashPBKDF2(clear, generateSalt(), DEFAULT_PBKDF2_ITERATIONS, DEFAULT_PBKDF2_OUTPUT_LENGTH, ENABLED_ALGORITHM)
  }

  return "", errors.New("invalid default hash algorithm: unknown hash method")

}

func HashBCRYPT(clear string, cost int) (string, error) {
  encrypted, err := bcrypt.GenerateFromPassword([]byte(clear), cost)
  if(err != nil) {
    return "", err
  }
  return "{BCRYPT}" + string(encrypted[:]), nil
}

func HashPBKDF2(clear string, salt []byte, iterations int, keyLength int, algorithm string) (string, error) {
  hashAlgorithm, err := getHashAlgorithm(algorithm)
  if(err != nil) {
    return "", errors.New("invalid default hash algorithm: unknown hash algorithm")
  }
  output := []string{
    "{" + algorithm + "}",
    strconv.Itoa(iterations),
    base64.StdEncoding.EncodeToString(salt),
    base64.StdEncoding.EncodeToString(encrypt(clear, salt, iterations, keyLength, hashAlgorithm)),
  }
  return strings.Join(output, "$"), nil
}

func getHashAlgorithm(algorithm string) (func() hash.Hash, error) {
  
  switch strings.ToUpper(algorithm) {
  case "PBKDF2-HMAC-SHA256":
    return sha256.New, nil
  case "PBKDF2-HMAC-SHA512":
    return sha512.New, nil
  }

  return nil, errors.New("cannot getHashAlgorithm from unknown algorithm: " + algorithm)

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

func getMethod(algorithm string) string {
  firstSeparatorIndex := strings.Index(algorithm, "-")
  if firstSeparatorIndex < 1 {
    return algorithm
  }
  return algorithm[0:firstSeparatorIndex]
}
