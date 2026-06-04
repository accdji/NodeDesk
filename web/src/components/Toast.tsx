import { useToastStore } from '../stores/toastStore';
import type { Toast } from '../stores/toastStore';

export function ToastContainer() {
  const toasts = useToastStore((s: { toasts: Toast[] }) => s.toasts);
  const remove = useToastStore((s: { remove: (id: string) => void }) => s.remove);

  if (toasts.length === 0) return null;

  return (
    <div className="toast-container">
      {toasts.map((t: Toast) => (
        <div
          key={t.id}
          className={`toast toast-${t.type}`}
          onClick={() => remove(t.id)}
          style={{ cursor: 'pointer' }}
        >
          {t.message}
        </div>
      ))}
    </div>
  );
}
