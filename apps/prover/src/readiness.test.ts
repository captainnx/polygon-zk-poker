import test from 'node:test';
import assert from 'node:assert/strict';
import { readiness, requireHandCircuit } from './readiness.ts';
test('missing build artifacts fail closed; production status stays false', () => {
  assert.equal(readiness.fullHandRelation, true);
  assert.equal(readiness.productionReady, false);
  assert.throws(()=>requireHandCircuit({...readiness,compiled:false}), /No proof was produced/);
});
