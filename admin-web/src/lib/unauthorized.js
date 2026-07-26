export function handleUnauthorizedResponse(status, actions) {
  if (Number(status) !== 401) {
    return false;
  }
  actions.clearSession();
  if (!String(actions.pathname || "").startsWith("/login")) {
    actions.redirect("/login");
  }
  return true;
}
