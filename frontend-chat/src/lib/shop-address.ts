export type ShopAddress = {
  postal_code: string;
  street: string;
  number: string;
  complement: string;
  district: string;
  city: string;
  state: string;
};

export const brazilianStates = [
  "AC",
  "AL",
  "AP",
  "AM",
  "BA",
  "CE",
  "DF",
  "ES",
  "GO",
  "MA",
  "MT",
  "MS",
  "MG",
  "PA",
  "PB",
  "PR",
  "PE",
  "PI",
  "RJ",
  "RN",
  "RS",
  "RO",
  "RR",
  "SC",
  "SP",
  "SE",
  "TO",
];

export function emptyShopAddress(): ShopAddress {
  return {
    postal_code: "",
    street: "",
    number: "",
    complement: "",
    district: "",
    city: "",
    state: "",
  };
}

export function validateShopAddress(
  address: ShopAddress,
): Partial<Record<keyof ShopAddress, string>> {
  const errors: Partial<Record<keyof ShopAddress, string>> = {};
  if (!/^[0-9]{8}$/.test(address.postal_code.replace(/-/g, "").trim()))
    errors.postal_code = "Informe os 8 números do CEP, por exemplo 01310-100.";
  for (const [field, max] of [
    ["street", 150],
    ["number", 20],
    ["complement", 100],
    ["district", 100],
    ["city", 100],
  ] as const) {
    const value = address[field];
    if (
      (field !== "complement" && !value.trim()) ||
      [...value.trim()].length > max ||
      /[\u0000-\u001f\u007f-\u009f]/.test(value)
    )
      errors[field] =
        field === "complement"
          ? `Use até ${max} caracteres, sem quebras de linha.`
          : `Preencha este campo com até ${max} caracteres, sem quebras de linha.`;
  }
  if (!brazilianStates.includes(address.state)) errors.state = "Selecione o estado da barbearia.";
  return errors;
}
