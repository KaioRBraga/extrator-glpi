package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestObterUsaCache(t *testing.T) {
	var chamadas int32
	c := Novo[int](time.Minute)
	carregar := func(context.Context) (int, error) {
		return int(atomic.AddInt32(&chamadas, 1)), nil
	}

	for i := 0; i < 3; i++ {
		v, err := c.Obter(context.Background(), "chave", carregar)
		if err != nil || v != 1 {
			t.Fatalf("valor = %d, erro = %v", v, err)
		}
	}
	if chamadas != 1 {
		t.Errorf("carregar chamado %d vezes, esperado 1", chamadas)
	}

	if _, err := c.Obter(context.Background(), "outra", carregar); err != nil {
		t.Fatal(err)
	}
	if chamadas != 2 {
		t.Errorf("chave nova deveria carregar de novo (chamadas = %d)", chamadas)
	}

	c.Limpar()
	if _, err := c.Obter(context.Background(), "chave", carregar); err != nil {
		t.Fatal(err)
	}
	if chamadas != 3 {
		t.Errorf("apos Limpar deveria recarregar (chamadas = %d)", chamadas)
	}
}

func TestObterExpira(t *testing.T) {
	var chamadas int32
	c := Novo[int](20 * time.Millisecond)
	carregar := func(context.Context) (int, error) {
		return int(atomic.AddInt32(&chamadas, 1)), nil
	}

	if _, err := c.Obter(context.Background(), "k", carregar); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := c.Obter(context.Background(), "k", carregar); err != nil {
		t.Fatal(err)
	}
	if chamadas != 2 {
		t.Errorf("carregar chamado %d vezes, esperado 2", chamadas)
	}
}

// Erro nao pode ser memorizado: a proxima tentativa precisa chamar o GLPI.
func TestErroNaoEhCacheado(t *testing.T) {
	var chamadas int32
	c := Novo[int](time.Minute)
	falha := errors.New("glpi fora do ar")
	carregar := func(context.Context) (int, error) {
		atomic.AddInt32(&chamadas, 1)
		return 0, falha
	}

	for i := 0; i < 2; i++ {
		if _, err := c.Obter(context.Background(), "k", carregar); !errors.Is(err, falha) {
			t.Fatalf("erro = %v", err)
		}
	}
	if chamadas != 2 {
		t.Errorf("carregar chamado %d vezes, esperado 2", chamadas)
	}
}

// Requisicoes simultaneas da mesma chave nao podem varrer o GLPI em paralelo.
func TestObterConcorrenteCarregaUmaVez(t *testing.T) {
	var chamadas int32
	c := Novo[int](time.Minute)
	carregar := func(context.Context) (int, error) {
		atomic.AddInt32(&chamadas, 1)
		time.Sleep(20 * time.Millisecond)
		return 42, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := c.Obter(context.Background(), "k", carregar); err != nil || v != 42 {
				t.Errorf("valor = %d, erro = %v", v, err)
			}
		}()
	}
	wg.Wait()
	if chamadas != 1 {
		t.Errorf("carregar chamado %d vezes, esperado 1", chamadas)
	}
}

func TestTTLZeroDesligaCache(t *testing.T) {
	var chamadas int32
	c := Novo[int](0)
	carregar := func(context.Context) (int, error) {
		return int(atomic.AddInt32(&chamadas, 1)), nil
	}
	for i := 0; i < 3; i++ {
		if _, err := c.Obter(context.Background(), "k", carregar); err != nil {
			t.Fatal(err)
		}
	}
	if chamadas != 3 {
		t.Errorf("carregar chamado %d vezes, esperado 3", chamadas)
	}
}
