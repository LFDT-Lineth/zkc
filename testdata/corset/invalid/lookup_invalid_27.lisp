;;error:3:36-40:conflicting context
(defcolumns (X :i16) (Y :i16))
(deflookup test ((_ X Y)) ((_ m1.A m2.B)))

(module m1)
(defcolumns (A :i16))

(module m2)
(defcolumns (B :i16))
