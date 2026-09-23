import type { Filtros } from "../api";

type Props = {
  valores: Filtros;
  aoMudar: (f: Filtros) => void;
  aoFiltrar: () => void;
  aoExportar: () => void;
  carregando: boolean;
  exportando: boolean;
};

const rotuloCampo = "mb-1 block text-sm font-semibold text-gray-200";
const campo =
  "w-full rounded-md border border-borda bg-fundo px-3 py-2 text-sm text-gray-100 " +
  "placeholder:text-gray-500 focus:border-acento focus:ring-2 focus:ring-acento/40 focus:outline-none";
const botao =
  "w-full rounded-md px-4 py-2 text-sm font-bold tracking-wide uppercase transition " +
  "disabled:cursor-not-allowed disabled:opacity-60";

export default function BarraFiltros({
  valores,
  aoMudar,
  aoFiltrar,
  aoExportar,
  carregando,
  exportando,
}: Props) {
  return (
    <form
      className="rounded-lg border border-borda bg-painel p-4 shadow-lg"
      onSubmit={(e) => {
        e.preventDefault();
        aoFiltrar();
      }}
    >
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-[1fr_1fr_2fr_1fr_auto]">
        <div>
          <label className={rotuloCampo} htmlFor="inicio">
            Data Inicial
          </label>
          <input
            id="inicio"
            type="date"
            className={campo}
            value={valores.inicio}
            max={valores.fim || undefined}
            onChange={(e) => aoMudar({ ...valores, inicio: e.target.value })}
          />
        </div>

        <div>
          <label className={rotuloCampo} htmlFor="fim">
            Data Final
          </label>
          <input
            id="fim"
            type="date"
            className={campo}
            value={valores.fim}
            min={valores.inicio || undefined}
            onChange={(e) => aoMudar({ ...valores, fim: e.target.value })}
          />
        </div>

        <div>
          <label className={rotuloCampo} htmlFor="busca">
            Buscar
          </label>
          <input
            id="busca"
            type="search"
            className={campo}
            placeholder="Deixe vazio para buscar tudo"
            value={valores.busca}
            onChange={(e) => aoMudar({ ...valores, busca: e.target.value })}
          />
        </div>

        <div>
          <label className={rotuloCampo} htmlFor="status">
            Status
          </label>
          <select
            id="status"
            className={campo}
            value={valores.status}
            onChange={(e) =>
              aoMudar({ ...valores, status: e.target.value as Filtros["status"] })
            }
          >
            <option value="todos">Todos</option>
            <option value="resolvidos">Resolvidos</option>
            <option value="abertos">Em aberto</option>
          </select>
        </div>

        <div className="flex flex-col justify-end gap-2 lg:w-36">
          <button
            type="submit"
            className={`${botao} bg-acento text-gray-900 hover:bg-orange-400`}
            disabled={carregando}
          >
            {carregando ? "Buscando..." : "Filtrar"}
          </button>
          <button
            type="button"
            onClick={aoExportar}
            className={`${botao} border border-acento text-acento hover:bg-acento-fraco`}
            disabled={carregando || exportando}
          >
            {exportando ? "Gerando..." : "Exportar"}
          </button>
        </div>
      </div>
    </form>
  );
}
