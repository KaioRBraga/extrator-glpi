import type { Stats } from "../api";

type Props = { stats: Stats; carregando: boolean };

/** Um numero grande com o rotulo em cima, como no painel atual. */
function Card({
  rotulo,
  valor,
  detalhe,
  destaque,
}: {
  rotulo: string;
  valor: string;
  detalhe?: string;
  destaque?: boolean;
}) {
  return (
    <div className="rounded-lg border border-borda bg-painel p-4 shadow-lg">
      <p className="text-xs font-bold tracking-widest text-gray-400 uppercase">{rotulo}</p>
      <p
        className={`mt-2 text-3xl font-bold tabular-nums ${
          destaque ? "text-acento" : "text-gray-100"
        }`}
      >
        {valor}
      </p>
      {detalhe && <p className="mt-1 text-xs text-gray-400">{detalhe}</p>}
    </div>
  );
}

export default function Cards({ stats, carregando }: Props) {
  const numero = (n: number) => (carregando ? "--" : n.toLocaleString("pt-BR"));
  const percentual = carregando
    ? "--"
    : `${stats.percentual_resolvidos.toLocaleString("pt-BR", {
        minimumFractionDigits: 1,
        maximumFractionDigits: 1,
      })}%`;

  return (
    <section className="mt-6">
      <h2 className="mb-3 text-lg font-bold text-gray-300">Estatísticas rápidas</h2>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        <Card rotulo="Total de chamados" valor={numero(stats.total)} detalhe="no período consultado" />
        <Card rotulo="Chamados filtrados" valor={numero(stats.filtrados)} detalhe="após status e busca" />
        <Card rotulo="Chamados fechados" valor={numero(stats.fechados)} detalhe={`${numero(stats.abertos)} em aberto`} />
        <Card rotulo="% Resolvidos" valor={percentual} detalhe="sobre os filtrados" destaque />
        <Card rotulo="Alta prioridade" valor={numero(stats.alta_prioridade)} detalhe="prioridade alta ou maior" />
      </div>

      {/* Barra de progresso: a leitura do percentual fica imediata. */}
      <div className="mt-4 h-2 w-full overflow-hidden rounded-full bg-borda">
        <div
          className="h-full rounded-full bg-acento transition-[width] duration-500"
          style={{ width: `${carregando ? 0 : stats.percentual_resolvidos}%` }}
          role="progressbar"
          aria-valuenow={stats.percentual_resolvidos}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label="Percentual de chamados resolvidos"
        />
      </div>
    </section>
  );
}
