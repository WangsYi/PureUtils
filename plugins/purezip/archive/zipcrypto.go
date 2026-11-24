package archive

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"hash"
	"hash/crc32"
	"io"
)

// ---------------------------------------------------------------------------
// Traditional PKWARE encryption (ZipCrypto)

type zipCryptoKeys struct{ k0, k1, k2 uint32 }

func newZipCryptoKeys(password string) *zipCryptoKeys {
	k := &zipCryptoKeys{0x12345678, 0x23456789, 0x34567890}
	for i := 0; i < len(password); i++ {
		k.update(password[i])
	}
	return k
}

func crc32Byte(crc uint32, b byte) uint32 {
	return crc32.IEEETable[byte(crc)^b] ^ (crc >> 8)
}

func (k *zipCryptoKeys) update(b byte) {
	k.k0 = crc32Byte(k.k0, b)
	k.k1 = (k.k1+(k.k0&0xff))*134775813 + 1
	k.k2 = crc32Byte(k.k2, byte(k.k1>>24))
}

func (k *zipCryptoKeys) stream() byte {
	t := uint16(k.k2) | 2
	return byte((uint32(t) * uint32(t^1)) >> 8)
}

func (k *zipCryptoKeys) decrypt(p []byte) {
	for i := range p {
		c := p[i] ^ k.stream()
		k.update(c)
		p[i] = c
	}
}

func (k *zipCryptoKeys) encrypt(p []byte) {
	for i := range p {
		c := p[i]
		p[i] = c ^ k.stream()
		k.update(c)
	}
}

type zipCryptoReader struct {
	r io.Reader
	k *zipCryptoKeys
}

func (z *zipCryptoReader) Read(p []byte) (int, error) {
	n, err := z.r.Read(p)
	z.k.decrypt(p[:n])
	return n, err
}

// newZipCryptoReader validates the 12-byte header against check and returns
// a decrypting reader of the remaining data.
func newZipCryptoReader(raw io.Reader, password string, check byte) (io.Reader, error) {
	k := newZipCryptoKeys(password)
	var hdr [12]byte
	if _, err := io.ReadFull(raw, hdr[:]); err != nil {
		return nil, err
	}
	k.decrypt(hdr[:])
	if hdr[11] != check {
		return nil, ErrWrongPassword
	}
	return &zipCryptoReader{r: raw, k: k}, nil
}

// zipCryptoEncrypt returns header+ciphertext for data.
func zipCryptoEncrypt(password string, check byte, data []byte, w io.Writer) error {
	k := newZipCryptoKeys(password)
	var hdr [12]byte
	if _, err := rand.Read(hdr[:11]); err != nil {
		return err
	}
	hdr[11] = check
	k.encrypt(hdr[:])
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	buf := make([]byte, 64<<10)
	for len(data) > 0 {
		n := copy(buf, data)
		k.encrypt(buf[:n])
		if _, err := w.Write(buf[:n]); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

// ---------------------------------------------------------------------------
// WinZip AES (AE-1 / AE-2)

const winzipAESExtraID = 0x9901

type aesExtra struct {
	version  uint16 // 1 = AE-1, 2 = AE-2
	strength byte   // 1=128, 2=192, 3=256
	method   uint16 // actual compression method
}

func parseAESExtra(extra []byte) (aesExtra, bool) {
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra)
		size := int(binary.LittleEndian.Uint16(extra[2:]))
		extra = extra[4:]
		if size > len(extra) {
			break
		}
		if id == winzipAESExtraID && size >= 7 {
			d := extra[:size]
			return aesExtra{version: binary.LittleEndian.Uint16(d), strength: d[4], method: binary.LittleEndian.Uint16(d[5:])}, true
		}
		extra = extra[size:]
	}
	return aesExtra{}, false
}

func buildAESExtra(method uint16) []byte {
	b := make([]byte, 11)
	binary.LittleEndian.PutUint16(b, winzipAESExtraID)
	binary.LittleEndian.PutUint16(b[2:], 7)
	binary.LittleEndian.PutUint16(b[4:], 2) // AE-2: CRC not stored
	b[6], b[7] = 'A', 'E'
	b[8] = 3 // AES-256
	binary.LittleEndian.PutUint16(b[9:], method)
	return b
}

func aesKeyLen(strength byte) int {
	switch strength {
	case 1:
		return 16
	case 2:
		return 24
	default:
		return 32
	}
}

func aesDeriveKeys(password string, salt []byte, keyLen int) (enc, auth, verify []byte) {
	dk, _ := pbkdf2.Key(sha1.New, password, salt, 1000, 2*keyLen+2)
	return dk[:keyLen], dk[keyLen : 2*keyLen], dk[2*keyLen:]
}

// winzipCTR is AES in CTR mode with WinZip's little-endian counter starting at 1.
type winzipCTR struct {
	block cipher.Block
	ctr   [16]byte
	ks    [16]byte
	pos   int
}

func newWinzipCTR(key []byte) (*winzipCTR, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &winzipCTR{block: b, pos: 16}, nil
}

func (c *winzipCTR) xor(p []byte) {
	for i := range p {
		if c.pos == 16 {
			for j := 0; j < 16; j++ {
				c.ctr[j]++
				if c.ctr[j] != 0 {
					break
				}
			}
			c.block.Encrypt(c.ks[:], c.ctr[:])
			c.pos = 0
		}
		p[i] ^= c.ks[c.pos]
		c.pos++
	}
}

// aesReader decrypts a WinZip AES payload of known length and verifies its
// HMAC-SHA1 authentication code at EOF.
type aesReader struct {
	r      io.Reader
	remain int64
	ctr    *winzipCTR
	mac    hash.Hash
	done   bool
}

func newAESReader(raw io.Reader, rawSize int64, password string, strength byte) (io.Reader, error) {
	keyLen := aesKeyLen(strength)
	saltLen := keyLen / 2
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(raw, salt); err != nil {
		return nil, err
	}
	var pv [2]byte
	if _, err := io.ReadFull(raw, pv[:]); err != nil {
		return nil, err
	}
	enc, auth, verify := aesDeriveKeys(password, salt, keyLen)
	if subtle.ConstantTimeCompare(pv[:], verify) != 1 {
		return nil, ErrWrongPassword
	}
	ctr, err := newWinzipCTR(enc)
	if err != nil {
		return nil, err
	}
	remain := rawSize - int64(saltLen) - 2 - 10
	if remain < 0 {
		return nil, ErrCorrupt
	}
	return &aesReader{r: raw, remain: remain, ctr: ctr, mac: hmac.New(sha1.New, auth)}, nil
}

func (a *aesReader) Read(p []byte) (int, error) {
	if a.remain <= 0 {
		if !a.done {
			a.done = true
			var tag [10]byte
			if _, err := io.ReadFull(a.r, tag[:]); err != nil {
				return 0, err
			}
			if !hmac.Equal(tag[:], a.mac.Sum(nil)[:10]) {
				return 0, errors.New("AES 校验失败（数据损坏或密码错误）")
			}
		}
		return 0, io.EOF
	}
	if int64(len(p)) > a.remain {
		p = p[:a.remain]
	}
	n, err := a.r.Read(p)
	if n > 0 {
		a.mac.Write(p[:n])
		a.ctr.xor(p[:n])
		a.remain -= int64(n)
	}
	if err == io.EOF && a.remain > 0 {
		return n, io.ErrUnexpectedEOF
	}
	if err == io.EOF {
		err = nil
	}
	return n, err
}

// aesEncrypt writes salt, verifier, ciphertext and tag for compressed data.
func aesEncrypt(password string, data []byte, w io.Writer) (int64, error) {
	const keyLen = 32
	salt := make([]byte, keyLen/2)
	if _, err := rand.Read(salt); err != nil {
		return 0, err
	}
	enc, auth, verify := aesDeriveKeys(password, salt, keyLen)
	ctr, err := newWinzipCTR(enc)
	if err != nil {
		return 0, err
	}
	mac := hmac.New(sha1.New, auth)
	var out bytes.Buffer
	out.Grow(len(data) + 28)
	out.Write(salt)
	out.Write(verify)
	start := out.Len()
	out.Write(data)
	ct := out.Bytes()[start:]
	ctr.xor(ct)
	mac.Write(ct)
	out.Write(mac.Sum(nil)[:10])
	n, err := w.Write(out.Bytes())
	return int64(n), err
}

// crcReader verifies a CRC-32 at EOF.
type crcReader struct {
	r    io.Reader
	h    hash.Hash32
	want uint32
	n    int64
	size int64
}

func (c *crcReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.h.Write(p[:n])
	c.n += int64(n)
	if err == io.EOF {
		if c.size >= 0 && c.n != c.size {
			return n, io.ErrUnexpectedEOF
		}
		if c.h.Sum32() != c.want {
			return n, errors.New("CRC 校验失败")
		}
	}
	return n, err
}
