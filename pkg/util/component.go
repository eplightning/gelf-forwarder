package util

import (
	"github.com/Graylog2/go-gelf/gelf"
	"sync"
)

type Component interface {
	Start() error
	Listen(msgCh chan *gelf.Message, stopCh chan interface{}) error
}

func RegisterComponent(
	component Component,
	wg *sync.WaitGroup,
	msgCh chan *gelf.Message,
	stopCh chan interface{},
	errCh chan error,
) error {
	if err := component.Start(); err != nil {
		return err
	}

	wg.Add(1)

	go func() {
		if err := component.Listen(msgCh, stopCh); err != nil {
			// TODO: log error
			errCh <- err
		}

		wg.Done()
	}()

	return nil
}
