export default function Modal({ open, title, children, onClose, footer, width = 760 }) {
  if (!open) return null;
  return (
    <div className="modal-layer" role="dialog" aria-modal="true">
      <div className="modal-backdrop" onClick={onClose} />
      <div className="modal-panel" style={{ maxWidth: `${width}px` }}>
        <div className="modal-header">
          <div>
            <h3>{title}</h3>
          </div>
          <button className="ghost-button" type="button" onClick={onClose}>
            关闭
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer ? <div className="modal-footer">{footer}</div> : null}
      </div>
    </div>
  );
}