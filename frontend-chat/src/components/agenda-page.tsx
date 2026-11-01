"use client";
import { useEffect, useState } from "react";
import { LoaderCircle, Plus, RefreshCw } from "lucide-react";
import { addDays, dateKey, parseDate, startOfWeek } from "@/lib/model";
import type { AdminAppointment } from "@/lib/api/agenda";
import { useSelectedShop } from "./shop-panel";
import { Alert, Button, Modal, PageHeader } from "./ui";
import { AdminBookingForm } from "./admin-booking-form";
import { useAgenda } from "./agenda/use-agenda";
import { AgendaCalendar, type AgendaView } from "./agenda/calendar";
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
  const [view, setView] = useState<AgendaView>("week");
  const [focusDay, setFocusDay] = useState("");
  const [selling, setSelling] = useState<AdminAppointment | null>(null);
  const [creating, setCreating] = useState(false);
  const [creationLocked, setCreationLocked] = useState(false);
  const [creationDirty, setCreationDirty] = useState(false);
  const [discarding, setDiscarding] = useState(false);
  useEffect(() => {
    if (loading) setSelected(null);
  }, [loading]);
  const focus = focusDay || data?.today || "";

  // The API loads one month grid (six full weeks); fetch another only when the period leaves it.
  function navigate(day: string, nextView: AgendaView = view) {
    setFocusDay(day);
    if (!data) return;
    const covered = (key: string) => key >= data.from_date && key < data.until_date;
    const weekStart = startOfWeek(parseDate(day));
    const needsMonth =
      nextView === "month"
        ? day.slice(0, 7) !== data.month
        : nextView === "week"
          ? !covered(dateKey(weekStart)) || !covered(dateKey(addDays(weekStart, 6)))
          : !covered(day);
    if (needsMonth) {
      setSelected(null);
      showMonth(day.slice(0, 7));
    }
  }
  function changeView(next: AgendaView) {
    setView(next);
    navigate(focus, next);
  }
  const newAppointment = (
    <>
      <Plus size={18} aria-hidden="true" />
      Novo agendamento
    </>
  );
  return (
    <>
      <PageHeader
        title="Agenda"
        description={"Acompanhe e organize os atendimentos de " + shop.name + "."}
        actions={
          <Button className={styles.headerNew} onClick={() => setCreating(true)}>
            {newAppointment}
          </Button>
        }
      />
      <Button
        className={styles.fab}
        aria-label="Novo agendamento"
        onClick={() => setCreating(true)}
      >
        <Plus size={24} aria-hidden="true" />
      </Button>
      {error && (
        <Alert tone="error">
          {error} Nenhum agendamento foi alterado. Use Atualizar agenda para tentar novamente.
        </Alert>
      )}
      {!data && error && (
        <Button variant="secondary" className={styles.retry} onClick={refresh} disabled={loading}>
          <RefreshCw size={18} aria-hidden="true" />
          Atualizar agenda
        </Button>
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
            view={view}
            focus={focus}
            onViewChange={changeView}
            onNavigate={(day) => navigate(day)}
            onOpenDay={(day) => {
              setView("day");
              navigate(day, "day");
            }}
            onRefresh={refresh}
            onSelectAppointment={setSelected}
          />
          {data.next_cursor && (
            <div className={styles.partialBox}>
              <Alert className={styles.partial}>
                Esta lista ainda é parcial. Carregue mais agendamentos antes de concluir que um dia
                não tem reservas.
              </Alert>
              <Button variant="secondary" disabled={loading} onClick={loadMore}>
                Carregar mais agendamentos
              </Button>
            </div>
          )}
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
