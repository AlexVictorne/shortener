// Package pool предоставляет типобезопасный generic-пул объектов,
// реализующих интерфейс [Resetter].
//
// Пул оборачивает [sync.Pool] и автоматически вызывает Reset()
// перед возвратом объекта, что гарантирует чистое состояние
// при следующем использовании.
package pool

import "sync"

// Resetter описывает объекты, умеющие сбрасывать свое состояние в исходное.
type Resetter interface {
	Reset()
}

// Pool — типобезопасная обертка над [sync.Pool] для объектов типа T.
// T должен реализовывать интерфейс [Resetter].
type Pool[T Resetter] struct {
	p sync.Pool
}

// New создает и возвращает указатель на новый [Pool].
// factory — фабричная функция, которую [sync.Pool] вызывает
// при отсутствии свободных объектов в пуле.
func New[T Resetter](factory func() T) *Pool[T] {
	return &Pool[T]{
		p: sync.Pool{New: func() any { return factory() }},
	}
}

// Get извлекает объект из пула или создает новый через фабрику,
// если пул пуст.
func (p *Pool[T]) Get() T {
	return p.p.Get().(T)
}

// Put сбрасывает состояние объекта v, вызывая v.Reset(),
// а затем возвращает его в пул для дальнейшего использования.
func (p *Pool[T]) Put(v T) {
	v.Reset()
	p.p.Put(v)
}
