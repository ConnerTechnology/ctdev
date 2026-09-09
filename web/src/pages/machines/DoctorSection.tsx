import type { DoctorRun } from '@/api/types';

export function DoctorSection({ runs }: { runs: DoctorRun[] }) {
  return (
    <section>
      <h2 className="mb-3 text-lg font-semibold">Doctor</h2>
      <p className="text-sm text-muted-foreground">{runs.length} runs recorded.</p>
    </section>
  );
}
