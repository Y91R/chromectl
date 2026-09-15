// Package entity — доменные сущности. Пакет не знает ни про транспорт, ни про
// хранилище, ни про сгенерированный код: он описывает предметную область и её инварианты.
//
// ADR: docs/adr/0005-domennyy-sloy-vmesto-dto.md — почему сущность, а не структура dto.
package entity

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/your-org/chrome_skill/internal/service/errors"
)

// ItemID — именованный тип, а не голый uuid.UUID: перепутать его с любым другим
// идентификатором становится невозможно на этапе компиляции.
type ItemID uuid.UUID

func NewItemID() ItemID           { return ItemID(uuid.New()) }
func (id ItemID) String() string  { return uuid.UUID(id).String() }
func (id ItemID) UUID() uuid.UUID { return uuid.UUID(id) }

func ParseItemID(s string) (ItemID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return ItemID{}, errors.Validation("id", "должен быть UUID")
	}
	return ItemID(id), nil
}

const maxNameLen = 255

type Item struct {
	id        ItemID
	name      string
	createdAt time.Time
}

// validateName держит инвариант имени в одном месте: и конструктор, и Rename
// обязаны проверять одно и то же, иначе правило обходится через второй вход.
// Длина считается в символах, как maxLength в спеке, а не в байтах: иначе
// кириллическое имя отвергается вдвое раньше обещанной границы.
func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.Validation("name", "не может быть пустым")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return "", errors.Validation("name", "длиннее допустимых 255 символов")
	}
	return name, nil
}

// NewItem — единственный способ создать корректный элемент: инвариант проверяется
// здесь, а не в транспорте, поэтому обойти его нельзя ни через HTTP, ни из воркера.
func NewItem(name string, now time.Time) (*Item, error) {
	name, err := validateName(name)
	if err != nil {
		return nil, err
	}
	return &Item{id: NewItemID(), name: name, createdAt: now}, nil
}

// RestoreItem собирает сущность из хранилища без проверки инвариантов: данные
// уже прошли их при создании, а повторная проверка сломала бы чтение старых записей.
func RestoreItem(id ItemID, name string, createdAt time.Time) *Item {
	return &Item{id: id, name: name, createdAt: createdAt}
}

func (i *Item) ID() ItemID           { return i.id }
func (i *Item) Name() string         { return i.name }
func (i *Item) CreatedAt() time.Time { return i.createdAt }

func (i *Item) Rename(name string) error {
	name, err := validateName(name)
	if err != nil {
		return err
	}
	i.name = name
	return nil
}
