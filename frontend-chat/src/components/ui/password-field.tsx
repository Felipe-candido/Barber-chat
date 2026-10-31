"use client";

import { useState, type ComponentProps } from "react";
import { Eye, EyeOff } from "lucide-react";
import { TextField } from "./form-field";

// Uses the same label, hint and error contract as every other text field.
export function PasswordField({
  visibilityLabel,
  disabled,
  reserveErrorSpace = true,
  ...props
}: Omit<ComponentProps<typeof TextField>, "type" | "endAdornment"> & { visibilityLabel: string }) {
  const [visible, setVisible] = useState(false);
  return (
    <TextField
      {...props}
      reserveErrorSpace={reserveErrorSpace}
      disabled={disabled}
      type={visible ? "text" : "password"}
      endAdornment={
        <button
          type="button"
          className="icon-button"
          disabled={disabled}
          aria-label={`${visible ? "Ocultar" : "Mostrar"} ${visibilityLabel}`}
          aria-pressed={visible}
          onClick={() => setVisible((value) => !value)}
        >
          {visible ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}
        </button>
      }
    />
  );
}
