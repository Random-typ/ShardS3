package interfaces

/*
*
*
*
*
*
 */

type DiscordService struct{}

func (s *DiscordService) GetMaxObjectSize() int {
	// Implement the method
	return 0
}

func (s *DiscordService) GetObject(location string) ([]byte, error) {
	// Implement the method
	return nil, nil
}

func (s *DiscordService) PutObject(data []byte) (string, error) {
	// Implement the method
	return "", nil
}

func (s *DiscordService) DeleteObject(location string) error {
	// Implement the method
	return nil
}
