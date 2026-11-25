;; Permutation whose source column is defined using a conditional.
(defcolumns (X :i16) (Y :i16) (IdX :i16) (one :i1 :constant 1))
;; IdX holds the value of X (as both branches of the conditional agree).
(defconstraint idx_def () (if (== X 0) (== IdX X) (== IdX X)))
(defpermutation test ((one IdX)) ((one Y)))
