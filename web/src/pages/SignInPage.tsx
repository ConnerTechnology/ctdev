import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { useSession } from '@/session/SessionContext';

const accounts = [
  { email: 'thomas@example.invalid', name: 'Thomas Conner' },
  { email: 'family-member@example.invalid', name: 'Family member' },
  { email: 'contractor@example.invalid', name: 'Contractor' },
  { email: 'stranger@example.invalid', name: 'stranger (not invited)' },
];

export function SignInPage() {
  const { signIn } = useSession();
  const navigate = useNavigate();
  const location = useLocation() as { state?: { from?: string } };
  const [choosing, setChoosing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function choose(email: string) {
    setError(null);
    try {
      await signIn(email);
      navigate(location.state?.from ?? '/machines', { replace: true });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <main className="flex min-h-svh items-center justify-center bg-background px-6">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center gap-3">
          <span className="flex h-9 w-9 items-center justify-center rounded bg-foreground font-display text-base font-semibold text-background">
            ct
          </span>
          <div>
            <h1 className="text-xl font-semibold">ctdev</h1>
            <p className="text-sm text-muted-foreground">Conner Technology machines</p>
          </div>
        </div>
        {!choosing ? (
          <Button className="w-full" onClick={() => setChoosing(true)}>
            Continue with Google
          </Button>
        ) : (
          <div className="rounded-md border bg-card" role="group" aria-label="Choose an account">
            {accounts.map((a) => (
              <button
                key={a.email}
                type="button"
                onClick={() => choose(a.email)}
                className="flex w-full flex-col items-start border-b px-4 py-3 text-left last:border-b-0 hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
              >
                <span className="text-sm">{a.name}</span>
                <span className="font-mono text-xs text-muted-foreground">{a.email}</span>
              </button>
            ))}
          </div>
        )}
        {error && (
          <p
            role="alert"
            className="mt-4 rounded-md border border-danger/40 bg-card px-3 py-2 text-sm"
          >
            {error}
          </p>
        )}
        <p className="mt-6 text-xs text-muted-foreground">
          Sign in with the Google account an admin invited. Conner Technology staff accounts are
          recognised automatically.
        </p>
      </div>
    </main>
  );
}
