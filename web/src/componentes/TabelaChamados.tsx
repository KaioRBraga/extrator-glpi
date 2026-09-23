import type { Chamado } from "../api";

type Props = {
  chamados: Chamado[];
  carregando: boolean;
  pagina: number;
  totalPaginas: number;
  aoMudarPagina: (p: number) => void;
};

const colunas = [
  "ID",
  "Título",
  "Status",
  "Abertura",
  "Solução",
  "Atendimento",
  "Motivo de abertura",
  "Motivo de encerramento",
  "Usuário de rede",
  "Requerente",
  "Técnico",
];

function Etiqueta({ chamado }: { chamado: Chamado }) {
  const cor = chamado.resolvido
    ? "bg-emerald-500/15 text-emerald-300 border-emerald-500/30"
    : "bg-amber-500/15 text-amber-300 border-amber-500/30";
  return (
    <span className={`inline-block rounded-full border px-2 py-0.5 text-xs whitespace-nowrap ${cor}`}>
      {chamado.status || (chamado.resolvido ? "Resolvido" : "Em aberto")}
    </span>
  );
}

export default function TabelaChamados({
  chamados,
  carregando,
  pagina,
  totalPaginas,
  aoMudarPagina,
}: Props) {
  return (
    <section className="mt-6">
      <div className="overflow-x-auto rounded-lg border border-borda bg-painel shadow-lg">
        <table className="w-full min-w-[74rem] border-collapse text-left text-sm">
          <thead>
            <tr className="border-b border-borda bg-black/20">
              {colunas.map((c) => (
                <th key={c} className="px-3 py-3 text-xs font-bold tracking-wider text-gray-400 uppercase">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {chamados.length === 0 && (
              <tr>
                <td colSpan={colunas.length} className="px-3 py-10 text-center text-gray-400">
                  {carregando ? (
                    <>
                      Consultando o GLPI...
                      <span className="mt-1 block text-xs text-gray-500">
                        A primeira consulta de um período leva cerca de 20 segundos. Depois dela,
                        filtrar e exportar são instantâneos.
                      </span>
                    </>
                  ) : (
                    "Nenhum chamado para os filtros atuais."
                  )}
                </td>
              </tr>
            )}

            {chamados.map((c) => (
              <tr key={c.id} className="border-b border-borda/60 last:border-0 hover:bg-white/5">
                <td className="px-3 py-2 font-mono text-xs whitespace-nowrap text-gray-300">{c.id}</td>
                <td className="max-w-[16rem] truncate px-3 py-2" title={c.titulo}>
                  {c.titulo}
                </td>
                <td className="px-3 py-2">
                  <Etiqueta chamado={c} />
                </td>
                <td className="px-3 py-2 whitespace-nowrap text-gray-300">
                  {c.data_abertura}
                  <span className="ml-1.5 text-xs text-gray-500">{c.hora_abertura}</span>
                </td>
                <td className="px-3 py-2 whitespace-nowrap text-gray-300">
                  {c.data_solucao ? (
                    <>
                      {c.data_solucao}
                      <span className="ml-1.5 text-xs text-gray-500">{c.hora_solucao}</span>
                    </>
                  ) : (
                    "—"
                  )}
                </td>
                {/* Tempo entre abertura e solucao, em HH:MM podendo passar de 24h. */}
                <td className="px-3 py-2 font-mono text-xs whitespace-nowrap text-gray-300" title="Tempo entre a abertura e a solução">
                  {c.atendimento || "—"}
                </td>
                <td className="max-w-[16rem] truncate px-3 py-2" title={`${c.motivo_abertura} (${c.origem_motivo})`}>
                  {c.motivo_abertura || "—"}
                </td>
                <td
                  className="max-w-[20rem] truncate px-3 py-2"
                  title={`${c.motivo_encerramento} (${c.origem_encerramento})`}
                >
                  {c.motivo_encerramento}
                </td>
                <td className="px-3 py-2 font-mono text-xs whitespace-nowrap text-gray-300">
                  {c.usuario_rede || "—"}
                </td>
                <td className="max-w-[12rem] truncate px-3 py-2 text-gray-300" title={c.requerente}>
                  {c.requerente || "—"}
                </td>
                <td className="max-w-[12rem] truncate px-3 py-2 text-gray-300" title={c.tecnico}>
                  {c.tecnico || "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {totalPaginas > 1 && (
        <div className="mt-3 flex items-center justify-end gap-3 text-sm">
          <button
            type="button"
            className="rounded-md border border-borda px-3 py-1 disabled:opacity-40"
            onClick={() => aoMudarPagina(pagina - 1)}
            disabled={pagina <= 1 || carregando}
          >
            Anterior
          </button>
          <span className="text-gray-400">
            Página {pagina} de {totalPaginas}
          </span>
          <button
            type="button"
            className="rounded-md border border-borda px-3 py-1 disabled:opacity-40"
            onClick={() => aoMudarPagina(pagina + 1)}
            disabled={pagina >= totalPaginas || carregando}
          >
            Próxima
          </button>
        </div>
      )}
    </section>
  );
}
