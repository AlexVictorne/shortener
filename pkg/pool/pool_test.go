package pool_test

import (
	"sync"
	"testing"

	"shortener/pkg/pool"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockObject — тестовый объект, реализующий интерфейс pool.Resetter.
type mockObject struct {
	Value      string
	ResetCalls int
}

func (m *mockObject) Reset() {
	m.Value = ""
	m.ResetCalls++
}

// factory создает новый mockObject для использования в пуле.
func factory() *mockObject {
	return &mockObject{}
}

func TestNew(t *testing.T) {
	p := pool.New(factory)
	require.NotNil(t, p)
}

func TestGet_ReturnsObject(t *testing.T) {
	p := pool.New(factory)

	obj := p.Get()

	require.NotNil(t, obj)
	assert.IsType(t, &mockObject{}, obj)
}

func TestGet_UsesFactory(t *testing.T) {
	callCount := 0
	p := pool.New(func() *mockObject {
		callCount++
		return &mockObject{}
	})

	_ = p.Get()
	_ = p.Get()

	assert.GreaterOrEqual(t, callCount, 1, "factory must be called at least once")
}

func TestPut_CallsReset(t *testing.T) {
	p := pool.New(factory)

	obj := p.Get()
	obj.Value = "some data"
	obj.ResetCalls = 0

	p.Put(obj)

	assert.Equal(t, 1, obj.ResetCalls, "Reset must be called exactly once on Put")
	assert.Empty(t, obj.Value, "Value must be cleared after Reset")
}

func TestGetAfterPut_ReturnsResetObject(t *testing.T) {
	p := pool.New(factory)

	obj := p.Get()
	obj.Value = "dirty"
	p.Put(obj)

	// sync.Pool не гарантирует возврат того же объекта,
	// но если он вернулся — его Value должно быть пустым.
	got := p.Get()
	assert.Empty(t, got.Value, "object from pool must have empty Value after Reset")
}

func TestPool_ConcurrentAccess(t *testing.T) {
	p := pool.New(factory)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			obj := p.Get()
			obj.Value = "concurrent"
			p.Put(obj)
		}()
	}

	wg.Wait()
}
