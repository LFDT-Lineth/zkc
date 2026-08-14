(defcolumns (X :i16) (Y :i16) (IdX :i16))
;; IdX holds the value of the expression (Id X)
(defconstraint idx_def () (if (== X 0) (== IdX X) (== IdX X)))
(deflookup test (Y) (IdX))
