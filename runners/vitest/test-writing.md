## Test Writing Guidelines (TypeScript / Vitest)

### File Location
- Co-located: `src/components/Button.test.tsx` next to `Button.tsx`
- Or in `__tests__/`: `src/components/__tests__/Button.test.tsx`
- The phase's `state.json` must have test files registered in `test_files[]`

### Test Structure
```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { myFunction } from './myModule';

describe('myFunction', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should return expected value for valid input', () => {
    const result = myFunction('input');
    expect(result).toBe('expected');
  });

  it('should throw on invalid input', () => {
    expect(() => myFunction(null)).toThrow('error message');
  });

  it('should handle async operations', async () => {
    const result = await myAsyncFunction();
    expect(result).toEqual({ key: 'value' });
  });
});
```

### Assertions
- `expect(value).toBe(expected)` — strict equality
- `expect(value).toEqual(expected)` — deep equality
- `expect(value).toBeTruthy()` / `.toBeFalsy()`
- `expect(fn).toThrow()` — expected errors
- `expect(value).toMatchSnapshot()` — snapshot testing

### Mocking
```typescript
// Mock a module
vi.mock('./api', () => ({
  fetchData: vi.fn().mockResolvedValue({ data: 'test' }),
}));

// Spy on a method
const spy = vi.spyOn(object, 'method');
expect(spy).toHaveBeenCalledWith('arg');
```

### Running Tests
```bash
arc iterate <plan> <phase> qa        # Write tests
arc iterate <plan> <phase> impl      # Implement to pass tests
```
