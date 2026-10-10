"use client";

import { useCallback, useState } from "react";
import { BookingForm } from "./booking-form";
import { useSelectedShop, useSelectedShopAccess } from "./shop-panel";
import { useServices } from "@/hooks/use-services";
import { money } from "@/lib/model";

export function AdminBookingForm({
  onLockedChange,
  onCreated,
}: {
  onLockedChange: (value: boolean) => void;
  onCreated: () => void;
}) {
  const shop = useSelectedShop();
  const access = useSelectedShopAccess();
  const { services, loading, error } = useServices(shop.slug, "admin");
  const [serviceID, setServiceID] = useState("");
  const [locked, setLocked] = useState(false);
  const handleLocked = useCallback(
    (value: boolean) => {
      setLocked(value);
      onLockedChange(value);
    },
    [onLockedChange],
  );
  const service = services.find((s) => s.id === serviceID);
  if (loading) return <p role="status">Carregando serviços…</p>;
  if (error)
    return (
      <p className="form-error" role="alert">
        {error}
      </p>
    );
  return (
    <div className="booking-form-flow">
      {!service && (
        <>
          <p>Escolha o serviço para reservar um horário.</p>
          <label className="booking-field">
            Serviço
            <select
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
            </select>
          </label>
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
            onCreated,
          }}
        />
      )}
    </div>
  );
}
