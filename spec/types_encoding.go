// Code generated by fastssz. DO NOT EDIT.
// Hash: e1810ae38bacf8535cd8a4d2e66d02bf8d4359ba3af5309f0b24445da8d09611
// Version: 0.1.3
package spec

import (
	ssz "github.com/ferranbt/fastssz"
)

// MarshalSSZ ssz marshals the Operator object
func (o *Operator) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(o)
}

// MarshalSSZTo ssz marshals the Operator object to a target array
func (o *Operator) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(12)

	// Field (0) 'ID'
	dst = ssz.MarshalUint64(dst, o.ID)

	// Offset (1) 'PubKey'
	dst = ssz.WriteOffset(dst, offset)
	offset += len(o.PubKey)

	// Field (1) 'PubKey'
	if size := len(o.PubKey); size > 2048 {
		err = ssz.ErrBytesLengthFn("Operator.PubKey", size, 2048)
		return
	}
	dst = append(dst, o.PubKey...)

	return
}

// UnmarshalSSZ ssz unmarshals the Operator object
func (o *Operator) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 12 {
		return ssz.ErrSize
	}

	tail := buf
	var o1 uint64

	// Field (0) 'ID'
	o.ID = ssz.UnmarshallUint64(buf[0:8])

	// Offset (1) 'PubKey'
	if o1 = ssz.ReadOffset(buf[8:12]); o1 > size {
		return ssz.ErrOffset
	}

	if o1 < 12 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (1) 'PubKey'
	{
		buf = tail[o1:]
		if len(buf) > 2048 {
			return ssz.ErrBytesLength
		}
		if cap(o.PubKey) == 0 {
			o.PubKey = make([]byte, 0, len(buf))
		}
		o.PubKey = append(o.PubKey, buf...)
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the Operator object
func (o *Operator) SizeSSZ() (size int) {
	size = 12

	// Field (1) 'PubKey'
	size += len(o.PubKey)

	return
}

// HashTreeRoot ssz hashes the Operator object
func (o *Operator) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(o)
}

// HashTreeRootWith ssz hashes the Operator object with a hasher
func (o *Operator) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'ID'
	hh.PutUint64(o.ID)

	// Field (1) 'PubKey'
	{
		elemIndx := hh.Index()
		byteLen := uint64(len(o.PubKey))
		if byteLen > 2048 {
			err = ssz.ErrIncorrectListSize
			return
		}
		hh.Append(o.PubKey)
		hh.MerkleizeWithMixin(elemIndx, byteLen, (2048+31)/32)
	}

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the Operator object
func (o *Operator) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(o)
}

// MarshalSSZ ssz marshals the Reshare object
func (r *Reshare) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(r)
}

// MarshalSSZTo ssz marshals the Reshare object to a target array
func (r *Reshare) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(100)

	// Field (0) 'ValidatorPubKey'
	if size := len(r.ValidatorPubKey); size != 48 {
		err = ssz.ErrBytesLengthFn("Reshare.ValidatorPubKey", size, 48)
		return
	}
	dst = append(dst, r.ValidatorPubKey...)

	// Offset (1) 'OldOperators'
	dst = ssz.WriteOffset(dst, offset)
	for ii := 0; ii < len(r.OldOperators); ii++ {
		offset += 4
		offset += r.OldOperators[ii].SizeSSZ()
	}

	// Offset (2) 'NewOperators'
	dst = ssz.WriteOffset(dst, offset)
	for ii := 0; ii < len(r.NewOperators); ii++ {
		offset += 4
		offset += r.NewOperators[ii].SizeSSZ()
	}

	// Field (3) 'OldT'
	dst = ssz.MarshalUint64(dst, r.OldT)

	// Field (4) 'NewT'
	dst = ssz.MarshalUint64(dst, r.NewT)

	// Field (5) 'Owner'
	dst = append(dst, r.Owner[:]...)

	// Field (6) 'Nonce'
	dst = ssz.MarshalUint64(dst, r.Nonce)

	// Field (1) 'OldOperators'
	if size := len(r.OldOperators); size > 13 {
		err = ssz.ErrListTooBigFn("Reshare.OldOperators", size, 13)
		return
	}
	{
		offset = 4 * len(r.OldOperators)
		for ii := 0; ii < len(r.OldOperators); ii++ {
			dst = ssz.WriteOffset(dst, offset)
			offset += r.OldOperators[ii].SizeSSZ()
		}
	}
	for ii := 0; ii < len(r.OldOperators); ii++ {
		if dst, err = r.OldOperators[ii].MarshalSSZTo(dst); err != nil {
			return
		}
	}

	// Field (2) 'NewOperators'
	if size := len(r.NewOperators); size > 13 {
		err = ssz.ErrListTooBigFn("Reshare.NewOperators", size, 13)
		return
	}
	{
		offset = 4 * len(r.NewOperators)
		for ii := 0; ii < len(r.NewOperators); ii++ {
			dst = ssz.WriteOffset(dst, offset)
			offset += r.NewOperators[ii].SizeSSZ()
		}
	}
	for ii := 0; ii < len(r.NewOperators); ii++ {
		if dst, err = r.NewOperators[ii].MarshalSSZTo(dst); err != nil {
			return
		}
	}

	return
}

// UnmarshalSSZ ssz unmarshals the Reshare object
func (r *Reshare) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 100 {
		return ssz.ErrSize
	}

	tail := buf
	var o1, o2 uint64

	// Field (0) 'ValidatorPubKey'
	if cap(r.ValidatorPubKey) == 0 {
		r.ValidatorPubKey = make([]byte, 0, len(buf[0:48]))
	}
	r.ValidatorPubKey = append(r.ValidatorPubKey, buf[0:48]...)

	// Offset (1) 'OldOperators'
	if o1 = ssz.ReadOffset(buf[48:52]); o1 > size {
		return ssz.ErrOffset
	}

	if o1 < 100 {
		return ssz.ErrInvalidVariableOffset
	}

	// Offset (2) 'NewOperators'
	if o2 = ssz.ReadOffset(buf[52:56]); o2 > size || o1 > o2 {
		return ssz.ErrOffset
	}

	// Field (3) 'OldT'
	r.OldT = ssz.UnmarshallUint64(buf[56:64])

	// Field (4) 'NewT'
	r.NewT = ssz.UnmarshallUint64(buf[64:72])

	// Field (5) 'Owner'
	copy(r.Owner[:], buf[72:92])

	// Field (6) 'Nonce'
	r.Nonce = ssz.UnmarshallUint64(buf[92:100])

	// Field (1) 'OldOperators'
	{
		buf = tail[o1:o2]
		num, err := ssz.DecodeDynamicLength(buf, 13)
		if err != nil {
			return err
		}
		r.OldOperators = make([]*Operator, num)
		err = ssz.UnmarshalDynamic(buf, num, func(indx int, buf []byte) (err error) {
			if r.OldOperators[indx] == nil {
				r.OldOperators[indx] = new(Operator)
			}
			if err = r.OldOperators[indx].UnmarshalSSZ(buf); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Field (2) 'NewOperators'
	{
		buf = tail[o2:]
		num, err := ssz.DecodeDynamicLength(buf, 13)
		if err != nil {
			return err
		}
		r.NewOperators = make([]*Operator, num)
		err = ssz.UnmarshalDynamic(buf, num, func(indx int, buf []byte) (err error) {
			if r.NewOperators[indx] == nil {
				r.NewOperators[indx] = new(Operator)
			}
			if err = r.NewOperators[indx].UnmarshalSSZ(buf); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the Reshare object
func (r *Reshare) SizeSSZ() (size int) {
	size = 100

	// Field (1) 'OldOperators'
	for ii := 0; ii < len(r.OldOperators); ii++ {
		size += 4
		size += r.OldOperators[ii].SizeSSZ()
	}

	// Field (2) 'NewOperators'
	for ii := 0; ii < len(r.NewOperators); ii++ {
		size += 4
		size += r.NewOperators[ii].SizeSSZ()
	}

	return
}

// HashTreeRoot ssz hashes the Reshare object
func (r *Reshare) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(r)
}

// HashTreeRootWith ssz hashes the Reshare object with a hasher
func (r *Reshare) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'ValidatorPubKey'
	if size := len(r.ValidatorPubKey); size != 48 {
		err = ssz.ErrBytesLengthFn("Reshare.ValidatorPubKey", size, 48)
		return
	}
	hh.PutBytes(r.ValidatorPubKey)

	// Field (1) 'OldOperators'
	{
		subIndx := hh.Index()
		num := uint64(len(r.OldOperators))
		if num > 13 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range r.OldOperators {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 13)
	}

	// Field (2) 'NewOperators'
	{
		subIndx := hh.Index()
		num := uint64(len(r.NewOperators))
		if num > 13 {
			err = ssz.ErrIncorrectListSize
			return
		}
		for _, elem := range r.NewOperators {
			if err = elem.HashTreeRootWith(hh); err != nil {
				return
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 13)
	}

	// Field (3) 'OldT'
	hh.PutUint64(r.OldT)

	// Field (4) 'NewT'
	hh.PutUint64(r.NewT)

	// Field (5) 'Owner'
	hh.PutBytes(r.Owner[:])

	// Field (6) 'Nonce'
	hh.PutUint64(r.Nonce)

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the Reshare object
func (r *Reshare) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(r)
}

// MarshalSSZ ssz marshals the SignedReshare object
func (s *SignedReshare) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(s)
}

// MarshalSSZTo ssz marshals the SignedReshare object to a target array
func (s *SignedReshare) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(8)

	// Offset (0) 'Reshare'
	dst = ssz.WriteOffset(dst, offset)
	offset += s.Reshare.SizeSSZ()

	// Offset (1) 'Signature'
	dst = ssz.WriteOffset(dst, offset)
	offset += len(s.Signature)

	// Field (0) 'Reshare'
	if dst, err = s.Reshare.MarshalSSZTo(dst); err != nil {
		return
	}

	// Field (1) 'Signature'
	if size := len(s.Signature); size > 1536 {
		err = ssz.ErrBytesLengthFn("SignedReshare.Signature", size, 1536)
		return
	}
	dst = append(dst, s.Signature...)

	return
}

// UnmarshalSSZ ssz unmarshals the SignedReshare object
func (s *SignedReshare) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 8 {
		return ssz.ErrSize
	}

	tail := buf
	var o0, o1 uint64

	// Offset (0) 'Reshare'
	if o0 = ssz.ReadOffset(buf[0:4]); o0 > size {
		return ssz.ErrOffset
	}

	if o0 < 8 {
		return ssz.ErrInvalidVariableOffset
	}

	// Offset (1) 'Signature'
	if o1 = ssz.ReadOffset(buf[4:8]); o1 > size || o0 > o1 {
		return ssz.ErrOffset
	}

	// Field (0) 'Reshare'
	{
		buf = tail[o0:o1]
		if err = s.Reshare.UnmarshalSSZ(buf); err != nil {
			return err
		}
	}

	// Field (1) 'Signature'
	{
		buf = tail[o1:]
		if len(buf) > 1536 {
			return ssz.ErrBytesLength
		}
		if cap(s.Signature) == 0 {
			s.Signature = make([]byte, 0, len(buf))
		}
		s.Signature = append(s.Signature, buf...)
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the SignedReshare object
func (s *SignedReshare) SizeSSZ() (size int) {
	size = 8

	// Field (0) 'Reshare'
	size += s.Reshare.SizeSSZ()

	// Field (1) 'Signature'
	size += len(s.Signature)

	return
}

// HashTreeRoot ssz hashes the SignedReshare object
func (s *SignedReshare) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(s)
}

// HashTreeRootWith ssz hashes the SignedReshare object with a hasher
func (s *SignedReshare) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Reshare'
	if err = s.Reshare.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (1) 'Signature'
	{
		elemIndx := hh.Index()
		byteLen := uint64(len(s.Signature))
		if byteLen > 1536 {
			err = ssz.ErrIncorrectListSize
			return
		}
		hh.Append(s.Signature)
		hh.MerkleizeWithMixin(elemIndx, byteLen, (1536+31)/32)
	}

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the SignedReshare object
func (s *SignedReshare) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(s)
}

// MarshalSSZ ssz marshals the Proof object
func (p *Proof) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(p)
}

// MarshalSSZTo ssz marshals the Proof object to a target array
func (p *Proof) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(120)

	// Field (0) 'ValidatorPubKey'
	if size := len(p.ValidatorPubKey); size != 48 {
		err = ssz.ErrBytesLengthFn("Proof.ValidatorPubKey", size, 48)
		return
	}
	dst = append(dst, p.ValidatorPubKey...)

	// Offset (1) 'EncryptedShare'
	dst = ssz.WriteOffset(dst, offset)
	offset += len(p.EncryptedShare)

	// Field (2) 'SharePubKey'
	if size := len(p.SharePubKey); size != 48 {
		err = ssz.ErrBytesLengthFn("Proof.SharePubKey", size, 48)
		return
	}
	dst = append(dst, p.SharePubKey...)

	// Field (3) 'Owner'
	dst = append(dst, p.Owner[:]...)

	// Field (1) 'EncryptedShare'
	if size := len(p.EncryptedShare); size > 512 {
		err = ssz.ErrBytesLengthFn("Proof.EncryptedShare", size, 512)
		return
	}
	dst = append(dst, p.EncryptedShare...)

	return
}

// UnmarshalSSZ ssz unmarshals the Proof object
func (p *Proof) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 120 {
		return ssz.ErrSize
	}

	tail := buf
	var o1 uint64

	// Field (0) 'ValidatorPubKey'
	if cap(p.ValidatorPubKey) == 0 {
		p.ValidatorPubKey = make([]byte, 0, len(buf[0:48]))
	}
	p.ValidatorPubKey = append(p.ValidatorPubKey, buf[0:48]...)

	// Offset (1) 'EncryptedShare'
	if o1 = ssz.ReadOffset(buf[48:52]); o1 > size {
		return ssz.ErrOffset
	}

	if o1 < 120 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (2) 'SharePubKey'
	if cap(p.SharePubKey) == 0 {
		p.SharePubKey = make([]byte, 0, len(buf[52:100]))
	}
	p.SharePubKey = append(p.SharePubKey, buf[52:100]...)

	// Field (3) 'Owner'
	copy(p.Owner[:], buf[100:120])

	// Field (1) 'EncryptedShare'
	{
		buf = tail[o1:]
		if len(buf) > 512 {
			return ssz.ErrBytesLength
		}
		if cap(p.EncryptedShare) == 0 {
			p.EncryptedShare = make([]byte, 0, len(buf))
		}
		p.EncryptedShare = append(p.EncryptedShare, buf...)
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the Proof object
func (p *Proof) SizeSSZ() (size int) {
	size = 120

	// Field (1) 'EncryptedShare'
	size += len(p.EncryptedShare)

	return
}

// HashTreeRoot ssz hashes the Proof object
func (p *Proof) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(p)
}

// HashTreeRootWith ssz hashes the Proof object with a hasher
func (p *Proof) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'ValidatorPubKey'
	if size := len(p.ValidatorPubKey); size != 48 {
		err = ssz.ErrBytesLengthFn("Proof.ValidatorPubKey", size, 48)
		return
	}
	hh.PutBytes(p.ValidatorPubKey)

	// Field (1) 'EncryptedShare'
	{
		elemIndx := hh.Index()
		byteLen := uint64(len(p.EncryptedShare))
		if byteLen > 512 {
			err = ssz.ErrIncorrectListSize
			return
		}
		hh.Append(p.EncryptedShare)
		hh.MerkleizeWithMixin(elemIndx, byteLen, (512+31)/32)
	}

	// Field (2) 'SharePubKey'
	if size := len(p.SharePubKey); size != 48 {
		err = ssz.ErrBytesLengthFn("Proof.SharePubKey", size, 48)
		return
	}
	hh.PutBytes(p.SharePubKey)

	// Field (3) 'Owner'
	hh.PutBytes(p.Owner[:])

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the Proof object
func (p *Proof) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(p)
}

// MarshalSSZ ssz marshals the SignedProof object
func (s *SignedProof) MarshalSSZ() ([]byte, error) {
	return ssz.MarshalSSZ(s)
}

// MarshalSSZTo ssz marshals the SignedProof object to a target array
func (s *SignedProof) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	dst = buf
	offset := int(260)

	// Offset (0) 'Proof'
	dst = ssz.WriteOffset(dst, offset)
	if s.Proof == nil {
		s.Proof = new(Proof)
	}
	offset += s.Proof.SizeSSZ()

	// Field (1) 'Signature'
	if size := len(s.Signature); size != 256 {
		err = ssz.ErrBytesLengthFn("SignedProof.Signature", size, 256)
		return
	}
	dst = append(dst, s.Signature...)

	// Field (0) 'Proof'
	if dst, err = s.Proof.MarshalSSZTo(dst); err != nil {
		return
	}

	return
}

// UnmarshalSSZ ssz unmarshals the SignedProof object
func (s *SignedProof) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 260 {
		return ssz.ErrSize
	}

	tail := buf
	var o0 uint64

	// Offset (0) 'Proof'
	if o0 = ssz.ReadOffset(buf[0:4]); o0 > size {
		return ssz.ErrOffset
	}

	if o0 < 260 {
		return ssz.ErrInvalidVariableOffset
	}

	// Field (1) 'Signature'
	if cap(s.Signature) == 0 {
		s.Signature = make([]byte, 0, len(buf[4:260]))
	}
	s.Signature = append(s.Signature, buf[4:260]...)

	// Field (0) 'Proof'
	{
		buf = tail[o0:]
		if s.Proof == nil {
			s.Proof = new(Proof)
		}
		if err = s.Proof.UnmarshalSSZ(buf); err != nil {
			return err
		}
	}
	return err
}

// SizeSSZ returns the ssz encoded size in bytes for the SignedProof object
func (s *SignedProof) SizeSSZ() (size int) {
	size = 260

	// Field (0) 'Proof'
	if s.Proof == nil {
		s.Proof = new(Proof)
	}
	size += s.Proof.SizeSSZ()

	return
}

// HashTreeRoot ssz hashes the SignedProof object
func (s *SignedProof) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(s)
}

// HashTreeRootWith ssz hashes the SignedProof object with a hasher
func (s *SignedProof) HashTreeRootWith(hh ssz.HashWalker) (err error) {
	indx := hh.Index()

	// Field (0) 'Proof'
	if err = s.Proof.HashTreeRootWith(hh); err != nil {
		return
	}

	// Field (1) 'Signature'
	if size := len(s.Signature); size != 256 {
		err = ssz.ErrBytesLengthFn("SignedProof.Signature", size, 256)
		return
	}
	hh.PutBytes(s.Signature)

	hh.Merkleize(indx)
	return
}

// GetTree ssz hashes the SignedProof object
func (s *SignedProof) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(s)
}
