import { usePush } from '../hooks/usePush';

export function PushToggle() {
  const { status, subscribe, unsubscribe } = usePush();

  if (status === 'unsupported') {
    return (
      <p className="field-note">
        Push requires iOS 16.4+ with the app added to Home Screen, or Android Chrome.
      </p>
    );
  }

  if (status === 'blocked') {
    return (
      <p className="field-note" style={{ color: 'var(--color-err, #f87171)' }}>
        Notifications are blocked — enable them in browser/OS settings.
      </p>
    );
  }

  if (status === 'subscribed') {
    return (
      <div className="push-row">
        <span className="push-row__label">Push notifications enabled on this device</span>
        <button className="btn btn--ghost btn--sm" onClick={unsubscribe}>Disable</button>
      </div>
    );
  }

  return (
    <button className="btn btn--secondary" onClick={subscribe}>
      Enable Push Notifications
    </button>
  );
}
