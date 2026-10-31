"use client";

import { useCallback, useEffect, useState } from "react";
import { useAdminAccess } from "./admin-access-provider";
import { BookingForm } from "./booking-form";
import { useSelectedShop, useSelectedShopAccess } from "./shop-panel";
import { useServices } from "@/hooks/use-services";
import { money } from "@/lib/model";
import { Alert, SelectField } from "./ui";
import styles from "./booking/booking.module.css";

export function AdminBookingForm({
  onLockedChange,
  onCreated,
  onDirtyChange,
}: {
  onLockedChange: (value: boolean) => void;
  onCreated: () => void;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const shop = useSelectedShop();
  const access = useSelectedShopAccess();
  const { state } = useAdminAccess();
  const { services, loading, error } = useServices(shop.slug, "admin");
  const [serviceID, setServiceID] = useState("");
  const [locked, setLocked] = useState(false);
  const [completed, setCompleted] = useState(false);
  useEffect(() => {
    onDirtyChange(!!serviceID && !completed);
    return () => onDirtyChange(false);
  }, [serviceID, completed, onDirtyChange]);
  const handleLocked = useCallback(
    (value: boolean) => {
      setLocked(value);
      onLockedChange(value);
    },
    [onLockedChange],
  );
  const service = services.find((s) => s.id === serviceID);
  if (loading || state.status !== "ready") return <p role="status">Carregando serviços…</p>;
  if (error) return <Alert tone="error">{error}</Alert>;
  return (
    <div className={styles.flow}>
      {!service && (
        <>
          <p>Escolha o serviço para reservar um horário.</p>
          <SelectField
            label="Serviço"
            value={serviceID}
            onChange={(e) => setServiceID(e.target.value)}
            disabled={locked}
          >
            <option value="">Selecione um serviço</option>
            {services.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name} · {money(s.price_cents)}
              </option>
            ))}
          </SelectField>
          {!services.length && <p>Nenhum serviço ativo nesta barbearia.</p>}
        </>
      )}
      {service && (
        <BookingForm
          key={service.id}
          slug={shop.slug}
          service={service}
          onBack={() => {
            if (!locked) setServiceID("");
          }}
          onLockedChange={handleLocked}
          staff={{
            professionalID: access.role === "barber" ? access.professional_id : null,
            shopID: shop.shop_id,
            userID: state.identity.user_id,
            role: access.role,
            onCreated: () => {
              setCompleted(true);
              onCreated();
            },
          }}
        />
      )}
    </div>
  );
}
