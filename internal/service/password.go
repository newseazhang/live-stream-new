package service

import "github.com/alexedwards/argon2id"

func HashPassword(password string) (string, error) {
	params := argon2id.Params{
		Memory:      19 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}

	return argon2id.CreateHash(password, &params)
}
