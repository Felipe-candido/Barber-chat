"use client";
import { useEffect, useId, useState } from "react";
import { LoaderCircle, Plus, RefreshCw } from "lucide-react";
import { addDays, dateKey, formatDate, parseDate } from "@/lib/model";
import { agendaDate, type AdminAppointment } from "@/lib/api/agenda";
import { useSelectedShop } from "./shop-panel";
import { Alert, Button, Modal, PageHeader } from "./ui";
import { AdminBookingForm } from "./admin-booking-form";
import { useAgenda } from "./agenda/use-agenda";
import { AgendaCalendar } from "./agenda/calendar";
import { AppointmentList } from "./agenda/appointment-list";
import { AppointmentDetails } from "./agenda/appointment-details";
import { AppointmentLifecyclePanel } from "./agenda/appointment-lifecycle-panel";
import { SaleForm } from "./commerce/sale-form";
import styles from "./agenda/agenda.module.css";

export function AgendaPage() {
  const shop = useSelectedShop();
  const [selected, setSelected] = useState<AdminAppointment | null>(null);
  const [detailBusy, setDetailBusy] = useState(false);
  const { data, loading, error, refresh, showMonth, loadMore } = useAgenda(
    shop.slug,
    selected?.version !== undefined,
  );
  const [dayFilter, setDayFilter] = useState("");
  const [selling, setSelling] = useState<AdminAppointment | null>(null);
  const [creating, setCreating] = useState(false);
  const [creationLocked, setCreationLocked] = useState(false);
  const [creationDirty, setCreationDirty] = useState(false);
  const [discarding, setDiscarding] = useState(false);
  const listTitle = useId();
  useEffect(() => {
    if (loading) setSelected(null);
  }, [loading]);
  function changeMonth(value: string) {
    setDayFilter("");
    setSelected(null);
    showMonth(value);
  }
  const visible = (data?.appointments ?? [])
    .filter((item) => !dayFilter || agendaDate(item.starts_at, data!.shop_timezone) === dayFilter)
    .sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at));
  return (
    <>
      <PageHeader
        title="Agenda"
        description={"Acompanhe e organize os atendimentos de " + shop.name + "."}
        actions={
          <Button onClick={() => setCreating(true)}>
            <Plus size={18} aria-hidden="true" />
            Novo agendamento
          </Button>
        }
      />
      <div className={styles.actions}>
        <Button variant="secondary" onClick={refresh} disabled={loading}>
          <RefreshCw size={18} aria-hidden="true" />
          Atualizar agenda
        </Button>
        {data && <span>Horários em {data.shop_timezone}</span>}
      </div>
      {error && (
        <Alert tone="error">
          {error} Nenhum agendamento foi alterado. Use Atualizar agenda para tentar novamente.
        </Alert>
      )}
      {loading && (
        <p className={data ? styles.refreshing : styles.loading} role="status">
          <LoaderCircle size={22} aria-hidden="true" />
          Carregando agendamentos…
        </p>
      )}
      {data && (
        <>
          <AgendaCalendar
            data={data}
            loading={loading}
            selectedDay={dayFilter}
            onSelectDay={setDayFilter}
            onSelectAppointment={setSelected}
            onMonthChange={changeMonth}
            onToday={() => {
              changeMonth(data.today.slice(0, 7));
              setDayFilter(data.today);
            }}
          />
          <section className={styles.list} aria-labelledby={listTitle}>
            <div className={styles.listHeader}>
              <div>
                <h2 id={listTitle}>Agendamentos do período</h2>
                <p>
                  {dayFilter
                    ? formatDate(dayFilter, { day: "2-digit", month: "long", year: "numeric" })
                    : "Selecione um dia para filtrar os atendimentos."}
                </p>
              </div>
              <div className={styles.filter}>
                <label>
                  Filtrar por dia
                  <input
                    type="date"
                    value={dayFilter}
                    min={data.from_date}
                    max={dateKey(addDays(parseDate(data.until_date), -1))}
                    onChange={(event) => setDayFilter(event.target.value)}
                  />
                </label>
                {dayFilter && (
                  <Button variant="secondary" onClick={() => setDayFilter("")}>
                    Ver todo o período
                  </Button>
                )}
              </div>
            </div>
            {data.next_cursor && (
              <Alert className={styles.partial}>
                Esta lista ainda é parcial. Carregue mais agendamentos antes de concluir que um dia
                não tem reservas.
              </Alert>
            )}
            <AppointmentList
              items={visible}
              timezone={data.shop_timezone}
              filtered={!!dayFilter}
              partial={!!data.next_cursor}
              onSelect={setSelected}
            />
            {data.next_cursor && (
              <Button variant="secondary" disabled={loading} onClick={loadMore}>
                Carregar mais agendamentos
              </Button>
            )}
          </section>
        </>
      )}
      <Modal
        open={!!selected}
        onClose={() => {
          if (detailBusy) return;
          const update = selected?.version !== undefined;
          setSelected(null);
          if (update) refresh();
        }}
        title="Detalhes do agendamento"
        subtitle={shop.name}
      >
        {selected &&
          data &&
          (selected.version !== undefined ? (
            <AppointmentLifecyclePanel
              key={selected.id}
              id={selected.id}
              timezone={data.shop_timezone}
              onBusyChange={setDetailBusy}
              onSaved={() => {
                setSelected(null);
                refresh();
              }}
              onSell={(item) => {
                setSelling(item);
                setSelected(null);
              }}
            />
          ) : (
            <AppointmentDetails
              item={selected}
              timezone={data.shop_timezone}
              onSell={() => {
                setSelling(selected);
                setSelected(null);
              }}
            />
          ))}
      </Modal>
      {selling && <SaleForm appointment={selling} onClose={() => setSelling(null)} />}
      <Modal
        open={creating}
        onClose={() => {
          if (creationLocked) return;
          if (discarding) {
            setDiscarding(false);
            return;
          }
          if (creationDirty) {
            setDiscarding(true);
            return;
          }
          setCreating(false);
        }}
        title="Novo agendamento"
        subtitle="Escolha um horário livre e informe os dados do cliente."
        wide
      >
        {discarding && (
          <div role="alert">
            <p>
              Descartar os dados deste agendamento? Nenhum horário será reservado por esta ação.
            </p>
            <Button variant="secondary" onClick={() => setDiscarding(false)}>
              Continuar preenchendo
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                setDiscarding(false);
                setCreating(false);
              }}
            >
              Descartar agendamento
            </Button>
          </div>
        )}
        <div hidden={discarding}>
          <AdminBookingForm
            onLockedChange={setCreationLocked}
            onDirtyChange={setCreationDirty}
            onCreated={refresh}
          />
        </div>
      </Modal>
    </>
  );
}
