;; Permutation within a single (non-root) module, whose source column is defined
;; by an expression.
(module m1)
(defcolumns (X :i16) (Y :i17) (X2 :i17) (one :i1 :constant 1))
;; X2 holds the value of the expression (* X 2).
(defconstraint x2_def () (== X2 (* X 2)))
(defpermutation test ((one X2)) ((one Y)))
