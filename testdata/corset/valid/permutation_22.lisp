;; Permutation with a (non-constant) source selector.
(defcolumns (X :i16) (P :binary) (Y :i16) (one :i1 :constant 1))
(defpermutation test ((P X)) ((one Y)))
