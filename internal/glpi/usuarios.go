package glpi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Campos de busca do User.
const (
	campoUserLogin = "1"
	campoUserID    = "2"
	campoUserNome  = "9"  // firstname
	campoUserSobre = "34" // realname
)

const (
	// validadeUsuarios: o cadastro muda pouco, e recarregar custa uma requisicao.
	validadeUsuarios = 30 * time.Minute
	// loteUsuarios: o cadastro e leve (so login e nome), cabe em poucas idas.
	loteUsuarios = 500
)

// resolverUsuarios troca os IDs numericos de requerente e tecnico pelos nomes.
//
// A busca de chamados devolve esses campos como ID mesmo com expand_dropdowns
// ligado (ao contrario de status e categoria), entao a traducao e feita aqui,
// com um mapa carregado de uma vez so.
func (c *Client) resolverUsuarios(ctx context.Context, chamados []Chamado) {
	if !precisaResolver(chamados) {
		return
	}

	mapa, err := c.mapaUsuarios(ctx)
	if err != nil {
		// Sem o mapa, os IDs continuam visiveis -- melhor do que falhar a busca.
		c.log.Warn("nao foi possivel carregar os nomes de usuario", "erro", err)
		return
	}

	for i := range chamados {
		chamados[i].Requerente = traduzirLista(chamados[i].Requerente, mapa)
		chamados[i].Tecnico = traduzirLista(chamados[i].Tecnico, mapa)
	}
}

func precisaResolver(chamados []Chamado) bool {
	for _, ch := range chamados {
		if temIDNumerico(ch.Requerente) || temIDNumerico(ch.Tecnico) {
			return true
		}
	}
	return false
}

func temIDNumerico(campo string) bool {
	for _, parte := range strings.Split(campo, " | ") {
		if _, err := strconv.Atoi(strings.TrimSpace(parte)); err == nil {
			return true
		}
	}
	return false
}

func traduzirLista(campo string, mapa map[string]string) string {
	if campo == "" {
		return campo
	}
	partes := strings.Split(campo, " | ")
	for i, parte := range partes {
		parte = strings.TrimSpace(parte)
		if nome, ok := mapa[parte]; ok && nome != "" {
			partes[i] = nome
		} else {
			partes[i] = parte
		}
	}
	return strings.Join(partes, " | ")
}

// mapaUsuarios devolve o mapa id -> nome, carregando do GLPI quando vencido.
func (c *Client) mapaUsuarios(ctx context.Context) (map[string]string, error) {
	c.muUsuarios.Lock()
	defer c.muUsuarios.Unlock()

	if c.usuarios != nil && time.Now().Before(c.usuariosAte) {
		return c.usuarios, nil
	}

	mapa, err := c.carregarUsuarios(ctx)
	if err != nil {
		return nil, err
	}
	c.usuarios, c.usuariosAte = mapa, time.Now().Add(validadeUsuarios)
	c.log.Info("cadastro de usuarios carregado", "usuarios", len(mapa))
	return mapa, nil
}

func (c *Client) carregarUsuarios(ctx context.Context) (map[string]string, error) {
	mapa := make(map[string]string)
	inicio := 0

	for {
		params := url.Values{}
		for i, campo := range []string{campoUserID, campoUserLogin, campoUserNome, campoUserSobre} {
			params.Set(fmt.Sprintf("forcedisplay[%d]", i), campo)
		}
		params.Set("range", fmt.Sprintf("%d-%d", inicio, inicio+loteUsuarios-1))
		params.Set("expand_dropdowns", "0")

		corpo, status, err := c.get(ctx, "/search/User", params)
		if err != nil {
			return nil, err
		}
		if status == 204 || len(corpo) == 0 {
			break
		}

		var resposta struct {
			TotalCount int              `json:"totalcount"`
			Data       []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(corpo, &resposta); err != nil {
			return nil, &Erro{Operacao: "interpretando cadastro de usuarios", Detalhe: trecho(corpo)}
		}
		if len(resposta.Data) == 0 {
			break
		}

		for _, item := range resposta.Data {
			id := valor(item, campoUserID, "id")
			if id == "" {
				continue
			}
			mapa[id] = nomeDeUsuario(item)
		}

		inicio += len(resposta.Data)
		if len(resposta.Data) < loteUsuarios || (resposta.TotalCount > 0 && inicio >= resposta.TotalCount) {
			break
		}
	}
	return mapa, nil
}

// nomeDeUsuario monta "Nome Sobrenome"; sem eles, usa o login.
func nomeDeUsuario(item map[string]any) string {
	nome := TextoSimples(valor(item, campoUserNome, "firstname"))
	sobrenome := TextoSimples(valor(item, campoUserSobre, "realname"))

	completo := strings.TrimSpace(nome + " " + sobrenome)
	if completo != "" {
		return completo
	}
	return TextoSimples(valor(item, campoUserLogin, "name"))
}
