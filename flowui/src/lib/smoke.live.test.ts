import { describe, expect, it } from 'vitest';
import { RemoteStore } from './remote';

// Live check against a running `flowd` (docker compose up). Skipped unless
// FLOW_LIVE=1, so the default `npm test` never needs the server.
//   npm run test:live
describe.runIf(process.env.FLOW_LIVE === '1')('live core (grpc-web)', () => {
  it('lists projects, loads a snapshot and writes a verdict', async () => {
    const store = new RemoteStore();
    const projects = await store.projects();
    expect(projects.some((p) => p.id === 'prj-travel')).toBe(true);

    const nodes = await store.nodes('prj-travel');
    expect(nodes.length).toBeGreaterThan(0);

    const deps = await store.dependencies('prj-travel');
    expect(deps.length).toBeGreaterThan(0);

    await store.setVerdict('T-1042', 'accepted');
    const [node] = (await store.nodes('prj-travel')).filter((n) => n.id === 'T-1042');
    expect(node.verification?.human).toBe('accepted');
  }, 20000);
});
