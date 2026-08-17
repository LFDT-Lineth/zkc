(defpurefun (Id A B) (if (== A 0) (== B A) (== B A)))
;;
(defcolumns (X :i16) (Y :i16) (IdX :i16))
;; IdX holds the value of the expression (Id X)
(defconstraint idx_def () (Id X IdX))
(deflookup test (Y) (IdX))
