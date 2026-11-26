;;error:7:17-18:conflicting context
(module m1)
(defcolumns (X :i64))

(module m2)
(defcolumns (A :i1) (B :i64))
(deflookup m1 ((A m1.X)) ((_ m2.B)))
