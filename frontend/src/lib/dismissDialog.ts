/** Route backdrop clicks through the dialog's existing cancel handler. */
export function dismissDialog(dialog: HTMLDialogElement) {
  let startedOutside = false;
  const isOutside = (event: MouseEvent) => {
    const bounds = dialog.getBoundingClientRect();
    return (
      event.target === dialog &&
      (event.clientX < bounds.left ||
        event.clientX > bounds.right ||
        event.clientY < bounds.top ||
        event.clientY > bounds.bottom)
    );
  };
  const reset = () => (startedOutside = false);
  const pointerdown = (event: PointerEvent) => {
    startedOutside = event.isPrimary && event.button === 0 && isOutside(event);
  };
  const click = (event: MouseEvent) => {
    const dismiss = startedOutside && isOutside(event);
    reset();
    if (dismiss && dialog.open) {
      const cancelled = !dialog.dispatchEvent(new Event("cancel", { cancelable: true }));
      if (!cancelled) dialog.close();
    }
  };
  dialog.addEventListener("pointerdown", pointerdown);
  dialog.addEventListener("pointercancel", reset);
  dialog.addEventListener("click", click);
  return {
    destroy() {
      dialog.removeEventListener("pointerdown", pointerdown);
      dialog.removeEventListener("pointercancel", reset);
      dialog.removeEventListener("click", click);
    },
  };
}
