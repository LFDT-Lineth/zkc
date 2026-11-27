;; Unchecked permutation with a (non-constant) source selector, whose source
;; column is wider than its target.
(defcolumns (X :i17) (P :binary) (Y :i16) (one :i1 :constant 1))
(defpermutation (test :unchecked) ((P X)) ((one Y)))
