const navigationEvent = "workbench:navigate";

export function requestWorkbenchNavigation() {
  return window.dispatchEvent(new Event(navigationEvent, { cancelable: true }));
}

export function guardUnsavedChanges() {
  let leaving = false;
  const beforeUnload = (event: BeforeUnloadEvent) => {
    if (!leaving) { event.preventDefault(); event.returnValue = ""; }
  };
  const navigate = (event: Event) => {
    if (!window.confirm("有未保存的状态内容，确定离开？")) event.preventDefault();
    else leaving = true;
  };
  window.addEventListener("beforeunload", beforeUnload);
  window.addEventListener(navigationEvent, navigate);
  return () => {
    window.removeEventListener("beforeunload", beforeUnload);
    window.removeEventListener(navigationEvent, navigate);
  };
}
