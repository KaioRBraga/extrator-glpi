import { useCallback, useEffect, useRef, useState } from "react";
import {
  buscarChamados,
  exportarCSV,
  filtrosIniciais,
  type Chamado,
  type Filtros,
  type Stats,
} from "./api";
import BarraFiltros from "./componentes/BarraFiltros";
import Cards from "./componentes/Cards";
import TabelaChamados from "./componentes/TabelaChamados";

const statsVazias: Stats = {
  total: 0,
  filtrados: 0,
  fechados: 0,
  abertos: 0,
  alta_prioridade: 0,
  percentual_resolvidos: 0,
};

const POR_PAGINA = 50;

export default function App() {
  // "rascunho" e o que esta nos campos; "aplicados" e o que foi buscado.
  // Sem essa separacao, digitar na busca dispararia consulta ao GLPI a cada tecla.
  const [rascunho, setRascunho] = useState<Filtros>(filtrosIniciais);
  const [aplicados, setAplicados] = useState<Filtros>(filtrosIniciais);

  const [chamados, setChamados] = useState<Chamado[]>([]);
  const [stats, setStats] = useState<Stats>(statsVazias);
  const [pagina, setPagina] = useState(1);
  const [totalPaginas, setTotalPaginas] = useState(0);
  const [truncado, setTruncado] = useState(false);

  const [carregando, setCarregando] = useState(true);
  const [exportando, setExportando] = useState(false);
  const [erro, setErro] = useState("");

  const requisicao = useRef<AbortController | null>(null);

  const carregar = useCallback(async (filtros: Filtros, novaPagina: number) => {
    requisicao.current?.abort();
    const controle = new AbortController();
    requisicao.current = controle;

    setCarregando(true);
    setErro("");
    try {
      const r = await buscarChamados(filtros, novaPagina, POR_PAGINA, controle.signal);
      setChamados(r.dados);
      setStats(r.stats);
      setTotalPaginas(r.total_paginas);
      setTruncado(r.truncado);
    } catch (e) {
      if (e instanceof DOMException && e.name === "AbortError") return;
      setChamados([]);
      setStats(statsVazias);
      setTotalPaginas(0);
      setErro(e instanceof Error ? e.message : "Falha inesperada na consulta.");
    } finally {
      if (requisicao.current === controle) setCarregando(false);
    }
  }, []);

  useEffect(() => {
    void carregar(aplicados, pagina);
  }, [carregar, aplicados, pagina]);

  function aoFiltrar() {
    setPagina(1);
    setAplicados(rascunho);
    // Filtros iguais nao mudam o estado e nao disparariam o efeito: recarrega.
    if (JSON.stringify(rascunho) === JSON.stringify(aplicados)) {
      void carregar(rascunho, 1);
    }
  }

  async function aoExportar() {
    setExportando(true);
    setErro("");
    try {
      await exportarCSV(rascunho);
    } catch (e) {
      setErro(e instanceof Error ? e.message : "Falha ao gerar o CSV.");
    } finally {
      setExportando(false);
    }
  }

  return (
    <div className="mx-auto max-w-[110rem] px-4 py-6 sm:px-6">
      <header className="mb-5">
        <h1 className="text-2xl font-bold text-acento sm:text-3xl">Extrator de chamados - GLPI</h1>
        <p className="mt-1 text-sm text-gray-400">
          Filtre por período e status, confira os indicadores e exporte o CSV.
        </p>
      </header>

      <BarraFiltros
        valores={rascunho}
        aoMudar={setRascunho}
        aoFiltrar={aoFiltrar}
        aoExportar={aoExportar}
        carregando={carregando}
        exportando={exportando}
      />

      {erro && (
        <div
          role="alert"
          className="mt-4 rounded-lg border border-red-500/40 bg-red-500/10 px-4 py-3 text-sm text-red-200"
        >
          {erro}
        </div>
      )}

      {truncado && !erro && (
        <div className="mt-4 rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">
          O período consultado atingiu o teto de chamados do serviço. Estreite as datas para ter
          certeza de que nada ficou de fora.
        </div>
      )}

      <Cards stats={stats} carregando={carregando} />

      <TabelaChamados
        chamados={chamados}
        carregando={carregando}
        pagina={pagina}
        totalPaginas={totalPaginas}
        aoMudarPagina={setPagina}
      />

      <footer className="mt-8 text-center text-xs text-gray-500">
        Chamados sem encerramento saem como "Chamado em aberto" na coluna de motivo de
        encerramento.
      </footer>
    </div>
  );
}
