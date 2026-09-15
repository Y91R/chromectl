// Package errors — доменные ошибки. Транспорт переводит их в коды ответов и
// больше ни о чём не догадывается: соответствие «ошибка → код» задано здесь.
package errors

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound = errors.New("не найдено")
	ErrConflict = errors.New("конфликт состояния")
)

// ValidationError несёт имя поля: без него клиент видит «неверный запрос»
// и не понимает, что именно исправить.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func Validation(field, reason string) error {
	return &ValidationError{Field: field, Reason: reason}
}

func AsValidation(err error) (*ValidationError, bool) {
	var v *ValidationError
	ok := errors.As(err, &v)
	return v, ok
}

func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
func IsConflict(err error) bool { return errors.Is(err, ErrConflict) }
