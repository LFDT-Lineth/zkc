;; Permutation with (non-constant) source and target selectors.
(defcolumns (P :binary) (X :i16) (Q :binary) (Y :i16))
(defpermutation test ((Q X)) ((P Y)))
