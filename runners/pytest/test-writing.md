## Test Writing Guidelines (Python / pytest)

### File Location
- In `tests/` directory: `tests/test_module.py`
- Or co-located: `src/module/test_module.py`
- The phase's `state.json` must have test files registered in `test_files[]`

### Test Structure
```python
import pytest
from mypackage.module import my_function

class TestMyFunction:
    def test_basic_case(self):
        result = my_function("input")
        assert result == "expected"

    def test_error_case(self):
        with pytest.raises(ValueError, match="invalid"):
            my_function(None)

    @pytest.fixture
    def sample_data(self):
        return {"key": "value"}

    def test_with_fixture(self, sample_data):
        result = my_function(sample_data)
        assert result is not None
```

### Assertions
- `assert value == expected` — equality
- `assert value is None` — identity
- `assert isinstance(value, MyClass)` — type checking
- `pytest.raises(Exception)` — expected errors
- `pytest.approx(value, abs=0.01)` — floating point

### Mocking
```python
from unittest.mock import patch, MagicMock

@patch('mypackage.module.external_call')
def test_with_mock(mock_call):
    mock_call.return_value = "mocked"
    result = function_using_external()
    assert result == "mocked"
    mock_call.assert_called_once_with("arg")
```

### Running Tests
```bash
arc iterate <plan> <phase> qa        # Write tests
arc iterate <plan> <phase> impl      # Implement to pass tests
```
