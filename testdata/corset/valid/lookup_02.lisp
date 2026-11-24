(defcolumns (A :i16) (B :i8) (X :i16) (Y :i8) (one :i1 :constant 1))
;; Inclusion A,B into X,Y
(deflookup test ((one A B)) ((one X Y)))
