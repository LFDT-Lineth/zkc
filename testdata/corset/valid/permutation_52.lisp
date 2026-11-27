;; As permutation_25, but without selectors.
(defcolumns (X :i16) (Y :i16) (IdX :i16))
;; IdX holds the value of X (as both branches of the conditional agree).
(defconstraint idx_def () (if (== X 0) (== IdX X) (== IdX X)))
(defpermutation test ((_ IdX)) ((_ Y)))
