// Package cache guarda por um tempo curto o resultado das consultas ao GLPI.
//
// Listar, recalcular os cards e exportar o CSV usam o mesmo conjunto de dados:
// sem cache, cada clique repetiria a varredura completa no GLPI.
package cache

import (
	"context"
	"sync"
	"time"
)

type entrada[T any] struct {
	mu     sync.Mutex
	valor  T
	expira time.Time
	valido bool
}

// Cache e um mapa com expiracao, seguro para uso concorrente.
type Cache[T any] struct {
	ttl   time.Duration
	mu    sync.Mutex
	itens map[string]*entrada[T]
}

// Novo cria um cache com o TTL informado. TTL <= 0 desliga o cache.
func Novo[T any](ttl time.Duration) *Cache[T] {
	return &Cache[T]{ttl: ttl, itens: make(map[string]*entrada[T])}
}

// Obter devolve o valor em cache ou chama carregar.
//
// Chamadas concorrentes para a mesma chave esperam a primeira terminar, em vez
// de dispararem varreduras paralelas no GLPI.
func (c *Cache[T]) Obter(ctx context.Context, chave string, carregar func(context.Context) (T, error)) (T, error) {
	if c.ttl <= 0 {
		return carregar(ctx)
	}

	c.mu.Lock()
	e, ok := c.itens[chave]
	if !ok {
		e = &entrada[T]{}
		c.itens[chave] = e
		c.limparExpiradosLocked()
	}
	c.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.valido && time.Now().Before(e.expira) {
		return e.valor, nil
	}

	valor, err := carregar(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	e.valor, e.expira, e.valido = valor, time.Now().Add(c.ttl), true
	return valor, nil
}

// Limpar descarta tudo (usado quando o usuario pede dados frescos).
func (c *Cache[T]) Limpar() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.itens = make(map[string]*entrada[T])
}

// limparExpiradosLocked evita que o mapa cresca sem limite com combinacoes de
// periodo pouco usadas. Exige c.mu travado.
func (c *Cache[T]) limparExpiradosLocked() {
	if len(c.itens) <= 32 {
		return
	}
	agora := time.Now()
	for chave, e := range c.itens {
		if !e.mu.TryLock() {
			continue // em uso por outra requisicao
		}
		if e.valido && agora.After(e.expira) {
			delete(c.itens, chave)
		}
		e.mu.Unlock()
	}
}
