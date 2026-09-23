export type Chamado = {
  id: string;
  titulo: string;
  status: string;
  data_abertura: string;
  data_solucao: string;
  motivo_abertura: string;
  origem_motivo: string;
  motivo_encerramento: string;
  origem_encerramento: string;
  usuario_rede: string;
  requerente: string;
  tecnico: string;
  categoria: string;
  prioridade: string;
  entidade: string;
  resolvido: boolean;
  alta_prioridade: boolean;
};

export type Stats = {
  total: number;
  filtrados: number;
  fechados: number;
  abertos: number;
  alta_prioridade: number;
  percentual_resolvidos: number;
};

export type RespostaChamados = {
  dados: Chamado[];
  stats: Stats;
  pagina: number;
  por_pagina: number;
  total_paginas: number;
  truncado: boolean;
};

export type Filtros = {
  inicio: string;
  fim: string;
  busca: string;
  status: "todos" | "resolvidos" | "abertos";
};

function iso(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(
    d.getDate(),
  ).padStart(2, "0")}`;
}

/**
 * A tela abre no mes corrente, nao em "tudo".
 *
 * Sem periodo, a consulta varre o historico inteiro do GLPI -- sao centenas de
 * chamados por dia. Limpar as datas continua trazendo tudo, para quem quiser.
 */
export function filtrosIniciais(): Filtros {
  const hoje = new Date();
  return {
    inicio: iso(new Date(hoje.getFullYear(), hoje.getMonth(), 1)),
    fim: iso(hoje),
    busca: "",
    status: "todos",
  };
}

export function parametros(f: Filtros, extras: Record<string, string> = {}): URLSearchParams {
  const p = new URLSearchParams();
  if (f.inicio) p.set("inicio", f.inicio);
  if (f.fim) p.set("fim", f.fim);
  if (f.busca.trim()) p.set("q", f.busca.trim());
  if (f.status !== "todos") p.set("status", f.status);
  for (const [chave, valor] of Object.entries(extras)) p.set(chave, valor);
  return p;
}

/** Erro com a mensagem que o backend devolveu, ja em portugues. */
export class ErroAPI extends Error {}

export async function buscarChamados(
  f: Filtros,
  pagina: number,
  porPagina: number,
  sinal?: AbortSignal,
): Promise<RespostaChamados> {
  const url = `/api/chamados?${parametros(f, {
    pagina: String(pagina),
    por_pagina: String(porPagina),
  })}`;

  const resp = await fetch(url, { signal: sinal });
  if (!resp.ok) {
    throw new ErroAPI(await mensagemDeErro(resp));
  }
  return (await resp.json()) as RespostaChamados;
}

/**
 * Exporta o CSV pela propria API. O download usa blob (e nao um link direto)
 * para que uma falha do GLPI vire mensagem na tela, em vez de um arquivo com
 * o JSON de erro dentro.
 */
export async function exportarCSV(f: Filtros): Promise<void> {
  const resp = await fetch(`/api/export.csv?${parametros(f)}`);
  if (!resp.ok) {
    throw new ErroAPI(await mensagemDeErro(resp));
  }

  const blob = await resp.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = nomeDoArquivo(resp) ?? "chamados.csv";
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function nomeDoArquivo(resp: Response): string | null {
  const cabecalho = resp.headers.get("Content-Disposition");
  const achado = cabecalho?.match(/filename="([^"]+)"/);
  return achado ? achado[1] : null;
}

async function mensagemDeErro(resp: Response): Promise<string> {
  try {
    const corpo = await resp.json();
    if (corpo && typeof corpo.erro === "string") return corpo.erro;
  } catch {
    // resposta sem JSON: cai no texto padrao abaixo
  }
  return `Falha na consulta (HTTP ${resp.status}).`;
}
