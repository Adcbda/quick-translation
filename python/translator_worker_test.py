import unittest

from translator_worker import collapse_degenerate_repetition


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


if __name__ == "__main__":
    unittest.main()
