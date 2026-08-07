(defpurefun (Id A) (if (== A 0) A A))
;;
(defcolumns (X :i16) (Y :i16) (IdX :i16))
;; IdX holds the value of the expression (Id X)
(defconstraint idx_def () (== IdX (Id X)))
(deflookup test (Y) (IdX))
