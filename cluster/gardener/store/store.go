package store

type Store struct{}

func (s Store) IdleSize() (int64, error) {

	return 31 << 1, nil
}

func (s Store) ID() string {
	return ""
}

func (s Store) Type() string {
	return ""
}

func (s *Store) Alloc(host string, size int64) error {

	return nil
}
