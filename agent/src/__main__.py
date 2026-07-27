"""Allow `python -m agent.src.cli` invocation."""
from .cli import main
import sys

sys.exit(main())
