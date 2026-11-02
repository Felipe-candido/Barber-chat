// Read only callback metadata, never retain credentials or display the provider's
// arbitrary error_description. A fixed local destination prevents open redirects.
export function readConfirmationCallback() {
  const query = new URLSearchParams(window.location.search);
  const fragment = new URLSearchParams(window.location.hash.slice(1));
  const hasError = [query, fragment].some(
    (params) => params.has("error") || params.has("error_code") || params.has("error_description"),
  );
  return {
    present: hasError || query.has("code") || !!window.location.hash,
    hasError,
    hasCode: query.has("code"),
    errorCode: query.get("error_code") ?? fragment.get("error_code") ?? "",
  };
}

export function confirmationLinkError(code = "") {
  if (code === "otp_expired") {
    return "Não foi possível validar o link: ele expirou ou já foi usado. Solicite um novo link em Reenviar confirmação. Se já confirmou o e-mail, entre pelo login.";
  }
  return "Não foi possível validar o link. Ele pode ter expirado ou ter sido aberto em outro navegador. Solicite outra confirmação neste navegador; se já confirmou o e-mail, entre pelo login e continue o cadastro.";
}
