package interfaces

/*
*
*
*
*
*
 */

type TelegramService struct{}

func (s *TelegramService) GetMaxObjectSize() int {
	// Implement the method
	return 0
}

func (s *TelegramService) GetObject(location string) ([]byte, error) {
	// Implement the method
	return nil, nil
}

func (s *TelegramService) PutObject(data []byte) (string, error) {
	// Implement the method
	return "", nil
}

func (s *TelegramService) DeleteObject(location string) error {
	// Implement the method
	return nil
}
