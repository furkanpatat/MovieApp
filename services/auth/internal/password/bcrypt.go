// Package password implements domain.PasswordHasher with bcrypt.
package password

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type Bcrypt struct{ cost int }

func NewBcrypt(cost int) (*Bcrypt, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, fmt.Errorf("bcrypt cost %d out of range [%d, %d]", cost, bcrypt.MinCost, bcrypt.MaxCost)
	}
	return &Bcrypt{cost: cost}, nil
}

// Hash returns a salted bcrypt hash (the salt and cost are embedded in it).
func (b *Bcrypt) Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), b.cost)
	return string(h), err
}

// Compare returns nil only for a matching password.
func (b *Bcrypt) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
