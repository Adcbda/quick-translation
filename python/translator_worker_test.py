import sys
import unittest
from types import SimpleNamespace
from unittest.mock import patch

from translator_worker import collapse_degenerate_repetition, load_model


class CollapseDegenerateRepetitionTest(unittest.TestCase):
    def test_collapses_single_token_loop_for_short_source(self):
        self.assertEqual(
            collapse_degenerate_repetition([5359, 5359, 5359, 5359, 5359], source_token_count=1),
            [5359],
        )

    def test_collapses_repeated_multi_token_tail(self):
        self.assertEqual(
            collapse_degenerate_repetition([7, 8, 9, 8, 9, 8, 9], source_token_count=1),
            [7, 8, 9],
        )

    def test_keeps_intentional_repetition_when_output_did_not_expand_abnormally(self):
        self.assertEqual(
            collapse_degenerate_repetition([4, 5, 4, 5, 4, 5], source_token_count=3),
            [4, 5, 4, 5, 4, 5],
        )

    def test_keeps_a_pair_of_repeated_tokens(self):
        self.assertEqual(
            collapse_degenerate_repetition([12, 12], source_token_count=1),
            [12, 12],
        )


class LoadModelTest(unittest.TestCase):
    def test_local_worker_disables_hugging_face_network_access(self):
        calls = []

        class FakeTokenizer:
            @staticmethod
            def from_pretrained(model_id, **options):
                calls.append(("tokenizer", model_id, options))
                return object()

        class FakeLoadedModel:
            def to(self, device):
                self.device = device

            def eval(self):
                self.evaluated = True

        class FakeModel:
            @staticmethod
            def from_pretrained(model_id, **options):
                calls.append(("model", model_id, options))
                return FakeLoadedModel()

        fake_torch = SimpleNamespace(cuda=SimpleNamespace(is_available=lambda: False))
        fake_transformers = SimpleNamespace(
            AutoModelForSeq2SeqLM=FakeModel,
            AutoTokenizer=FakeTokenizer,
        )
        with patch.dict(sys.modules, {"torch": fake_torch, "transformers": fake_transformers}):
            load_model("example/model", "model-cache", local_files_only=True)

        self.assertEqual(len(calls), 2)
        for _, model_id, options in calls:
            self.assertEqual(model_id, "example/model")
            self.assertEqual(options["cache_dir"], "model-cache")
            self.assertTrue(options["local_files_only"])

    def test_incomplete_local_cache_returns_actionable_error(self):
        class MissingModel:
            @staticmethod
            def from_pretrained(*args, **kwargs):
                raise OSError("missing cached file")

        fake_torch = SimpleNamespace(cuda=SimpleNamespace(is_available=lambda: False))
        fake_transformers = SimpleNamespace(
            AutoModelForSeq2SeqLM=MissingModel,
            AutoTokenizer=MissingModel,
        )
        with patch.dict(sys.modules, {"torch": fake_torch, "transformers": fake_transformers}):
            with self.assertRaisesRegex(RuntimeError, "重新下载"):
                load_model("example/model", "model-cache", local_files_only=True)


if __name__ == "__main__":
    unittest.main()
